import json
import os
from unittest.mock import MagicMock, patch

import pytest
from redis import Redis

from rustic_ai.core.guild.agent import AgentSpec
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.core.messaging import MessagingConfig
from rustic_ai.forge.execution_engine import ForgeExecutionEngine


@pytest.fixture
def mock_redis():
    mock = MagicMock(spec=Redis)
    mock.connection_pool = MagicMock()
    mock.connection_pool.connection_kwargs = {"host": "localhost"}
    return mock


@pytest.fixture
def engine(mock_redis):
    with patch("rustic_ai.forge.execution_engine.Redis", return_value=mock_redis):
        eng = ForgeExecutionEngine(guild_id="guild-1", organization_id="org-1")
        return eng


def test_run_agent_success(engine, mock_redis):
    mock_redis.brpop.return_value = (
        b"queue_name",
        json.dumps(
            {
                "request_id": "test-req-1",
                "success": True,
                "node_id": "node-mock",
                "pid": 1234,
            }
        ).encode("utf-8"),
    )

    agent_spec = MagicMock(spec=AgentSpec)
    agent_spec.id = "agent-1"
    agent_spec.model_dump.return_value = {"id": "agent-1", "class_name": "test"}
    guild_spec = MagicMock(spec=GuildSpec)
    guild_spec.id = "guild-1"
    messaging_config = MagicMock(spec=MessagingConfig)
    messaging_config.model_dump.return_value = {
        "backend_module": "fake",
        "backend_class": "Fake",
        "backend_config": {},
    }
    messaging_config.backend_config = {}
    messaging_config.backend_class = "Fake"

    with patch(
        "rustic_ai.forge.execution_engine.uuid.uuid4", return_value="test-req-1"
    ):
        agent = engine.run_agent(
            guild_spec=guild_spec,
            agent_spec=agent_spec,
            messaging_config=messaging_config,
            machine_id=1,
            client_properties={"host": "localhost"},
        )

    mock_redis.lpush.assert_called_once()
    args, _ = mock_redis.lpush.call_args
    assert args[0] == "forge:control:requests"

    wrapper = json.loads(args[1])
    assert wrapper["command"] == "spawn"

    payload = wrapper["payload"]
    assert payload["request_id"] == "test-req-1"
    assert payload["organization_id"] == "org-1"
    assert payload["guild_id"] == "guild-1"
    assert payload["agent_spec"]["id"] == "agent-1"
    assert payload["client_properties"]["organization_id"] == "org-1"

    assert agent is not None
    assert agent.agent_spec.id == "agent-1"
    assert "agent-1" in engine._agents


def test_stop_agent_success(engine, mock_redis):
    mock_spec = MagicMock(spec=AgentSpec)
    mock_spec.id = "agent-2"
    engine._agents["agent-2"] = mock_spec

    mock_redis.brpop.return_value = (
        b"queue_name",
        json.dumps({"request_id": "test-req-stop", "success": True}).encode("utf-8"),
    )

    with patch(
        "rustic_ai.forge.execution_engine.uuid.uuid4", return_value="test-req-stop"
    ):
        engine.stop_agent(guild_id="guild-1", agent_id="agent-2")

    mock_redis.lpush.assert_called_once()
    args, _ = mock_redis.lpush.call_args
    wrapper = json.loads(args[1])
    assert wrapper["command"] == "stop"
    assert wrapper["payload"]["organization_id"] == "org-1"
    assert wrapper["payload"]["agent_id"] == "agent-2"

    assert "agent-2" not in engine._agents


def test_is_agent_running(engine, mock_redis):
    mock_redis.get.return_value = json.dumps({"state": "running"}).encode("utf-8")
    assert engine.is_agent_running("guild-1", "agent-3") is True

    mock_redis.get.return_value = json.dumps({"state": "restarting"}).encode("utf-8")
    assert engine.is_agent_running("guild-1", "agent-3") is False

    mock_redis.get.return_value = None
    assert engine.is_agent_running("guild-1", "agent-3") is False

    mock_redis.get.return_value = json.dumps({"state": "failed"}).encode("utf-8")
    assert engine.is_agent_running("guild-1", "agent-3") is False


def test_shutdown(engine, mock_redis):
    mock_a1 = MagicMock(spec=AgentSpec)
    mock_a1.id = "a1"
    engine._agents["a1"] = mock_a1

    mock_a2 = MagicMock(spec=AgentSpec)
    mock_a2.id = "a2"
    engine._agents["a2"] = mock_a2

    mock_redis.brpop.return_value = (
        b"q",
        json.dumps({"success": True}).encode("utf-8"),
    )

    engine.shutdown()

    assert mock_redis.lpush.call_count == 2
    assert len(engine._agents) == 0


def test_run_agent_preserves_explicit_client_org(engine, mock_redis):
    mock_redis.brpop.return_value = (
        b"queue_name",
        json.dumps(
            {
                "request_id": "test-req-2",
                "success": True,
                "node_id": "node-mock",
                "pid": 5678,
            }
        ).encode("utf-8"),
    )

    agent_spec = MagicMock(spec=AgentSpec)
    agent_spec.id = "agent-9"
    agent_spec.model_dump.return_value = {"id": "agent-9", "class_name": "test"}
    guild_spec = MagicMock(spec=GuildSpec)
    guild_spec.id = "guild-1"
    messaging_config = MagicMock(spec=MessagingConfig)
    messaging_config.model_dump.return_value = {
        "backend_module": "fake",
        "backend_class": "Fake",
        "backend_config": {},
    }
    messaging_config.backend_config = {}
    messaging_config.backend_class = "Fake"

    with patch(
        "rustic_ai.forge.execution_engine.uuid.uuid4", return_value="test-req-2"
    ):
        _ = engine.run_agent(
            guild_spec=guild_spec,
            agent_spec=agent_spec,
            messaging_config=messaging_config,
            machine_id=1,
            client_properties={"organization_id": "org-explicit"},
        )

    args, _ = mock_redis.lpush.call_args
    wrapper = json.loads(args[1])
    payload = wrapper["payload"]
    assert payload["organization_id"] == "org-1"
    assert payload["client_properties"]["organization_id"] == "org-explicit"


# ---------------------------------------------------------------------------
# NATS backend tests — ForgeExecutionEngine routes through _NATSControlClient
# when NATS_URL is set.  _NATSControlClient is mocked to avoid a real server.
# ---------------------------------------------------------------------------


def _make_agent_spec(agent_id="agent-nats-1"):
    spec = MagicMock(spec=AgentSpec)
    spec.id = agent_id
    spec.model_dump.return_value = {"id": agent_id, "class_name": "test.Agent"}
    return spec


def _make_messaging_config():
    cfg = MagicMock(spec=MessagingConfig)
    cfg.model_dump.return_value = {
        "backend_module": "m",
        "backend_class": "NATSMessagingBackend",
        "backend_config": {},
    }
    cfg.backend_config = {}
    cfg.backend_class = "NATSMessagingBackend"
    return cfg


@pytest.fixture
def nats_engine():
    """ForgeExecutionEngine using the NATS backend (mocked _NATSControlClient)."""
    env = {**os.environ, "NATS_URL": "nats://localhost:4222"}
    # Unset REDIS_HOST/PORT/DB to ensure Redis() is never called
    env.pop("REDIS_HOST", None)
    with patch.dict(os.environ, env, clear=True):
        with patch("rustic_ai.forge.execution_engine._NATSControlClient") as MockNATS:
            mock_nats = MockNATS.return_value
            eng = ForgeExecutionEngine(guild_id="guild-1", organization_id="org-1")
            assert eng._backend == "nats"
            eng._mock_nats = mock_nats
            yield eng


def test_nats_run_agent_success(nats_engine):
    mock_nats = nats_engine._mock_nats
    mock_nats.wait_response.return_value = json.dumps(
        {"success": True, "node_id": "node-1", "pid": 42}
    ).encode()

    agent = nats_engine.run_agent(
        guild_spec=MagicMock(spec=GuildSpec),
        agent_spec=_make_agent_spec("agent-nats-1"),
        messaging_config=_make_messaging_config(),
        machine_id=1,
    )

    assert agent is not None
    assert agent.agent_spec.id == "agent-nats-1"
    assert "agent-nats-1" in nats_engine._agents

    # push_command must be called with the control queue and JSON payload
    mock_nats.push_command.assert_called_once()
    queue_key, raw_payload = mock_nats.push_command.call_args[0]
    assert queue_key == "forge:control:requests"
    wrapper = json.loads(raw_payload)
    assert wrapper["command"] == "spawn"
    assert wrapper["payload"]["guild_id"] == "guild-1"
    assert wrapper["payload"]["organization_id"] == "org-1"

    mock_nats.wait_response.assert_called_once()


def test_nats_run_agent_spawn_failure(nats_engine):
    """Engine returns None and does not track agent when spawn reports failure."""
    nats_engine._mock_nats.wait_response.return_value = json.dumps(
        {"success": False, "error": "no node available"}
    ).encode()

    agent = nats_engine.run_agent(
        guild_spec=MagicMock(spec=GuildSpec),
        agent_spec=_make_agent_spec("agent-fail"),
        messaging_config=_make_messaging_config(),
        machine_id=1,
    )

    assert agent is None
    assert "agent-fail" not in nats_engine._agents


def test_nats_run_agent_timeout(nats_engine):
    """Engine returns None when wait_response times out (returns None)."""
    nats_engine._mock_nats.wait_response.return_value = None

    agent = nats_engine.run_agent(
        guild_spec=MagicMock(spec=GuildSpec),
        agent_spec=_make_agent_spec("agent-timeout"),
        messaging_config=_make_messaging_config(),
        machine_id=1,
    )

    assert agent is None
    assert "agent-timeout" not in nats_engine._agents


def test_nats_stop_agent(nats_engine):
    mock_nats = nats_engine._mock_nats
    mock_nats.wait_response.return_value = json.dumps({"success": True}).encode()

    spec = _make_agent_spec("agent-stop")
    nats_engine._agents["agent-stop"] = spec

    nats_engine.stop_agent("guild-1", "agent-stop")

    mock_nats.push_command.assert_called_once()
    queue_key, raw_payload = mock_nats.push_command.call_args[0]
    assert queue_key == "forge:control:requests"
    wrapper = json.loads(raw_payload)
    assert wrapper["command"] == "stop"
    assert wrapper["payload"]["agent_id"] == "agent-stop"
    assert "agent-stop" not in nats_engine._agents


def test_nats_is_agent_running_true(nats_engine):
    nats_engine._mock_nats.get_status.return_value = {"state": "running", "pid": 123}
    assert nats_engine.is_agent_running("guild-1", "agent-1") is True
    nats_engine._mock_nats.get_status.assert_called_once_with("guild-1", "agent-1")


def test_nats_is_agent_running_non_running_states(nats_engine):
    """Only 'running' state reports the agent as running."""
    for state in ("restarting", "failed", "stopped", "pending"):
        nats_engine._mock_nats.get_status.return_value = {"state": state}
        assert nats_engine.is_agent_running("guild-1", "agent-1") is False, (
            f"state={state!r} should not be considered running"
        )


def test_nats_is_agent_running_not_found(nats_engine):
    """get_status returning None means agent is not running."""
    nats_engine._mock_nats.get_status.return_value = None
    assert nats_engine.is_agent_running("guild-1", "agent-1") is False


def test_nats_shutdown_stops_all_agents_and_closes_client(nats_engine):
    mock_nats = nats_engine._mock_nats
    mock_nats.wait_response.return_value = json.dumps({"success": True}).encode()

    for aid in ("a1", "a2", "a3"):
        spec = _make_agent_spec(aid)
        nats_engine._agents[aid] = spec

    nats_engine.shutdown()

    assert mock_nats.push_command.call_count == 3
    assert len(nats_engine._agents) == 0
    mock_nats.close.assert_called_once()


def test_nats_backend_does_not_inherit_redis_config(nats_engine):
    """NATS backend must not inject redis_client into NATS messaging config."""
    mock_nats = nats_engine._mock_nats
    mock_nats.wait_response.return_value = json.dumps({"success": True}).encode()

    cfg = _make_messaging_config()
    nats_engine.run_agent(
        guild_spec=MagicMock(spec=GuildSpec),
        agent_spec=_make_agent_spec("agent-cfg"),
        messaging_config=cfg,
        machine_id=1,
    )

    # redis_client must NOT have been injected into the NATS messaging config
    assert "redis_client" not in cfg.backend_config
