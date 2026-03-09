import json
import logging
import os
import uuid
from typing import Any, Dict, List, Optional, Type

from redis import Redis

from rustic_ai.core.guild.agent import Agent, AgentSpec
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.core.guild.execution.execution_engine import ExecutionEngine
from rustic_ai.core.messaging import Client, MessageTrackingClient, MessagingConfig

logger = logging.getLogger(__name__)

CONTROL_QUEUE = "forge:control:requests"


class RemoteAgentProxy(Agent):
    """Lightweight proxy returned by ForgeExecutionEngine for remotely managed agents."""

    def __init__(self, spec: AgentSpec):
        self.agent_spec = spec

    async def run(self):
        pass


def _decode_redis(value: bytes | str) -> str:
    if isinstance(value, bytes):
        return value.decode("utf-8")
    return value


class ForgeExecutionEngine(ExecutionEngine):
    """
    Forge-specific implementation of the ExecutionEngine.
    Dispatches start/stop commands via a Redis control queue and reads statuses directly.
    """

    def __init__(self, guild_id: str, organization_id: str) -> None:
        super().__init__(guild_id, organization_id)
        host = os.environ.get("REDIS_HOST", "localhost")
        port = int(os.environ.get("REDIS_PORT", "6379"))
        db = int(os.environ.get("REDIS_DB", "0"))
        self._rdb = Redis(host=host, port=port, db=db)
        self._agents: Dict[str, AgentSpec] = {}

    def _send_command(self, command: str, payload: dict) -> Optional[dict]:
        """Sends a command to the control queue and waits for a response."""
        req_id = payload["request_id"]
        wrapper = {"command": command, "payload": payload}
        resp_key = f"forge:control:response:{req_id}"
        timeout = 30 if command == "spawn" else 10

        try:
            self._rdb.lpush(CONTROL_QUEUE, json.dumps(wrapper))
        except Exception as e:
            logger.error(f"Failed to push {command} request to Redis: {e}")
            return None

        try:
            res = self._rdb.brpop(resp_key, timeout=timeout)
            if not res:
                logger.error(f"Timeout waiting for {command} response on {resp_key}")
                return None
            return json.loads(_decode_redis(res[1]))
        except Exception as e:
            logger.error(f"Failed to read {command} response from Redis: {e}")
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

        # Inherit Redis connection parameters for the child agent
        if "redis_client" not in messaging_config.backend_config:
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
            logger.error(f"Agent spawn failed: {error_msg}")
            return None

        self._agents[agent_spec.id] = agent_spec
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
            logger.error(f"Agent stop failed remotely: {response.get('error')}")

        self._agents.pop(agent_id, None)

    def is_agent_running(self, guild_id: str, agent_id: str) -> bool:
        key = f"forge:agent:status:{guild_id}:{agent_id}"
        try:
            val = self._rdb.get(key)
            if not val:
                return False
            status_data = json.loads(_decode_redis(val))
            return status_data.get("state", "unknown") == "running"
        except Exception as e:
            logger.error(f"Failed to check agent status from Redis: {e}")
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
