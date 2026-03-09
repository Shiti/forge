import json
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
