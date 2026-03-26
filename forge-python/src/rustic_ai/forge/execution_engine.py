import asyncio
import json
import logging
import os
import re
import threading
import time
import uuid
from typing import Any, Dict, List, Optional, Type

import nats
import nats.errors
import nats.js.api
import nats.js.errors
from redis import Redis

from rustic_ai.core.guild.agent import Agent, AgentSpec
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.core.guild.execution.execution_engine import ExecutionEngine
from rustic_ai.core.messaging import Client, MessageTrackingClient, MessagingConfig

logger = logging.getLogger(__name__)

CONTROL_QUEUE = "forge:control:requests"
_CTRL_RESPONSE_STREAM = "CTRL_RESPONSES"
_AGENT_STATUS_KV = "agent-status"
_DEFAULT_ACCEPTED_WARMUP_SECONDS = 5.0


def _ctrl_sanitize(name: str) -> str:
    """Matches Go ctrlSanitize: replace ':', '.', '$' with '_'."""
    return name.replace(":", "_").replace(".", "_").replace("$", "_")


def _kv_sanitize(name: str) -> str:
    """Matches Go kvSanitize: allow only [a-zA-Z0-9-_.], replace others with '_'."""
    return re.sub(r"[^a-zA-Z0-9\-_.]", "_", name)


def _decode_redis(value: bytes | str) -> str:
    if isinstance(value, bytes):
        return value.decode("utf-8")
    return value


class _NATSControlClient:
    """Sync wrapper around async nats-py for control plane operations."""

    def __init__(self, url: str) -> None:
        self._loop = asyncio.new_event_loop()
        self._thread = threading.Thread(
            target=self._loop.run_forever, daemon=True, name="nats-ctrl-loop"
        )
        self._thread.start()
        self._nc = self._run(nats.connect(url))
        self._js = self._nc.jetstream()
        self._streams_lock = threading.Lock()
        self._streams: set = set()

    def _run(self, coro, timeout: int = 30):
        return asyncio.run_coroutine_threadsafe(coro, self._loop).result(
            timeout=timeout
        )

    def _ensure_stream(
        self,
        name: str,
        subjects: list,
        max_age: float,
        retention: Optional[str] = None,
    ) -> None:
        with self._streams_lock:
            if name in self._streams:
                return

        async def _create():
            try:
                await self._js.stream_info(name)
            except Exception:
                cfg = nats.js.api.StreamConfig(
                    name=name,
                    subjects=subjects,
                    max_age=max_age,
                    retention=retention,
                )
                try:
                    await self._js.add_stream(cfg)
                except Exception:
                    # Race condition: another process (Go) already created the stream
                    try:
                        await self._js.stream_info(name)
                    except Exception:
                        pass

        self._run(_create())
        with self._streams_lock:
            self._streams.add(name)

    def push_command(self, queue_key: str, payload: bytes) -> None:
        stream_name = "CTRL_" + _ctrl_sanitize(queue_key)
        subject = "ctrl." + _ctrl_sanitize(queue_key)
        self._ensure_stream(
            stream_name,
            [subject],
            300.0,
            nats.js.api.RetentionPolicy.WORK_QUEUE,
        )

        async def _publish():
            await self._js.publish(subject, payload)

        self._run(_publish())

    def wait_response(self, request_id: str, timeout: int) -> Optional[bytes]:
        self._ensure_stream(_CTRL_RESPONSE_STREAM, ["ctrl.response.*"], 60.0)
        subject = "ctrl.response." + _ctrl_sanitize(request_id)
        consumer_name = "resp-" + uuid.uuid4().hex[:12]

        async def _fetch():
            config = nats.js.api.ConsumerConfig(
                durable_name=consumer_name,
                filter_subject=subject,
                ack_policy=nats.js.api.AckPolicy.NONE,
                deliver_policy=nats.js.api.DeliverPolicy.ALL,
                max_deliver=1,
                inactive_threshold=10.0,
            )
            try:
                sub = await self._js.pull_subscribe(
                    subject,
                    durable=consumer_name,
                    stream=_CTRL_RESPONSE_STREAM,
                    config=config,
                )
            except Exception as e:
                logger.error("Failed to create pull subscriber for response: %s", e)
                return None
            try:
                msgs = await sub.fetch(1, timeout=float(timeout))
                return msgs[0].data if msgs else None
            except (nats.errors.TimeoutError, asyncio.TimeoutError):
                return None
            finally:
                try:
                    await sub.unsubscribe()
                except Exception:
                    pass
                try:
                    await self._js.delete_consumer(_CTRL_RESPONSE_STREAM, consumer_name)
                except Exception:
                    pass

        return self._run(_fetch(), timeout=timeout + 10)

    def get_status(self, guild_id: str, agent_id: str) -> Optional[dict]:
        key = _kv_sanitize(guild_id) + "." + _kv_sanitize(agent_id)

        async def _get():
            try:
                kv = await self._js.key_value(_AGENT_STATUS_KV)
            except Exception:
                return None
            try:
                entry = await kv.get(key)
                return json.loads(entry.value)
            except nats.js.errors.KeyNotFoundError:
                return None

        try:
            return self._run(_get())
        except Exception as e:
            logger.debug("Failed to get agent status from NATS KV: %s", e)
            return None

    def close(self) -> None:
        try:
            self._run(self._nc.drain(), timeout=5)
        except Exception:
            pass
        self._loop.call_soon_threadsafe(self._loop.stop)
        self._thread.join(timeout=2)


class RemoteAgentProxy(Agent):
    """Lightweight proxy returned by ForgeExecutionEngine for remotely managed agents."""

    def __init__(self, spec: AgentSpec):
        self.agent_spec = spec

    async def run(self):
        pass


class ForgeExecutionEngine(ExecutionEngine):
    """
    Forge-specific implementation of the ExecutionEngine.
    Dispatches start/stop commands via a control queue (Redis or NATS) and reads
    agent statuses directly from the corresponding backend store.
    """

    def __init__(self, guild_id: str, organization_id: str) -> None:
        super().__init__(guild_id, organization_id)
        self._agents: Dict[str, AgentSpec] = {}
        self._accepted_warmup_seconds = float(
            os.environ.get(
                "FORGE_AGENT_ACCEPTED_WARMUP_SECONDS",
                str(_DEFAULT_ACCEPTED_WARMUP_SECONDS),
            )
        )

        nats_url = os.environ.get("NATS_URL")
        if nats_url:
            self._backend = "nats"
            self._nats = _NATSControlClient(nats_url)
        else:
            self._backend = "redis"
            host = os.environ.get("REDIS_HOST", "localhost")
            port = int(os.environ.get("REDIS_PORT", "6379"))
            db = int(os.environ.get("REDIS_DB", "0"))
            self._rdb = Redis(host=host, port=port, db=db)

    def _send_command(self, command: str, payload: dict) -> Optional[dict]:
        """Sends a command to the control queue and waits for a response."""
        req_id = payload["request_id"]
        wrapper = {"command": command, "payload": payload}
        timeout = 30 if command == "spawn" else 10

        if self._backend == "nats":
            try:
                self._nats.push_command(CONTROL_QUEUE, json.dumps(wrapper).encode())
            except Exception as e:
                logger.error("Failed to push %s to NATS: %s", command, e)
                return None
            try:
                data = self._nats.wait_response(req_id, timeout)
                return json.loads(data) if data else None
            except Exception as e:
                logger.error("Failed to read %s response from NATS: %s", command, e)
                return None
        else:
            resp_key = f"forge:control:response:{req_id}"
            try:
                self._rdb.lpush(CONTROL_QUEUE, json.dumps(wrapper))
            except Exception as e:
                logger.error("Failed to push %s request to Redis: %s", command, e)
                return None
            try:
                res = self._rdb.brpop(resp_key, timeout=timeout)
                if not res:
                    logger.error(
                        "Timeout waiting for %s response on %s", command, resp_key
                    )
                    return None
                return json.loads(_decode_redis(res[1]))
            except Exception as e:
                logger.error("Failed to read %s response from Redis: %s", command, e)
                return None

    def run_agent(
        self,
        guild_spec: GuildSpec,
        agent_spec: AgentSpec,
        messaging_config: MessagingConfig,
        machine_id: int,
        client_type: Type[Client] = MessageTrackingClient,
        client_properties: Dict[str, Any] = None,
        default_topic: str = "default_topic",
    ) -> Optional[Agent]:
        req_id = str(uuid.uuid4())
        org_id = self.organization_id

        # Inherit Redis connection parameters for Redis backends only
        if (
            self._backend == "redis"
            and messaging_config.backend_class == "RedisMessagingBackend"
            and "redis_client" not in messaging_config.backend_config
        ):
            kwargs = self._rdb.connection_pool.connection_kwargs
            messaging_config.backend_config["redis_client"] = {
                "host": kwargs.get("host", "localhost"),
                "port": kwargs.get("port", 6379),
                "db": kwargs.get("db", 0),
            }

        merged_client_properties = dict(client_properties or {})
        if org_id and "organization_id" not in merged_client_properties:
            merged_client_properties["organization_id"] = org_id

        payload = {
            "request_id": req_id,
            "organization_id": org_id,
            "guild_id": self.guild_id,
            "agent_spec": agent_spec.model_dump(),
            "messaging_config": messaging_config.model_dump(),
            "machine_id": machine_id,
            "client_type": (
                client_type.__name__
                if hasattr(client_type, "__name__")
                else str(client_type)
            ),
            "client_properties": merged_client_properties,
        }

        response = self._send_command("spawn", payload)
        if not response or not response.get("success", False):
            error_msg = (response or {}).get("error", "unknown error")
            logger.error("Agent spawn failed: %s", error_msg)
            return None

        self._agents[agent_spec.id] = agent_spec
        self._wait_for_initial_warmup()
        return RemoteAgentProxy(agent_spec)

    def stop_agent(self, guild_id: str, agent_id: str) -> None:
        req_id = str(uuid.uuid4())
        payload = {
            "request_id": req_id,
            "organization_id": self.organization_id,
            "guild_id": guild_id,
            "agent_id": agent_id,
        }

        response = self._send_command("stop", payload)
        if response and not response.get("success", False):
            logger.error("Agent stop failed remotely: %s", response.get("error"))

        self._agents.pop(agent_id, None)

    def is_agent_running(self, guild_id: str, agent_id: str) -> bool:
        if self._backend == "nats":
            try:
                status = self._nats.get_status(guild_id, agent_id)
                if status is not None:
                    return status.get("state", "unknown") in {"starting", "running"}
                return guild_id == self.guild_id and agent_id in self._agents
            except Exception as e:
                logger.error("Failed to check agent status from NATS: %s", e)
                return False
        else:
            key = f"forge:agent:status:{guild_id}:{agent_id}"
            try:
                val = self._rdb.get(key)
                if val:
                    status_data = json.loads(_decode_redis(val))
                    return status_data.get("state", "unknown") in {
                        "starting",
                        "running",
                    }
                return guild_id == self.guild_id and agent_id in self._agents
            except Exception as e:
                logger.error("Failed to check agent status from Redis: %s", e)
                return False

    def get_agents_in_guild(self, guild_id: str) -> Dict[str, AgentSpec]:
        return dict(self._agents)

    def find_agents_by_name(self, guild_id: str, agent_name: str) -> List[AgentSpec]:
        return [spec for spec in self._agents.values() if spec.name == agent_name]

    def shutdown(self) -> None:
        """Stops all agents tracked by this engine instance."""
        agent_ids = list(self._agents.keys())
        for aid in agent_ids:
            self.stop_agent(self.guild_id, aid)
        if self._backend == "nats":
            self._nats.close()

    def _wait_for_initial_warmup(self) -> None:
        if self._accepted_warmup_seconds <= 0:
            return
        time.sleep(self._accepted_warmup_seconds)
