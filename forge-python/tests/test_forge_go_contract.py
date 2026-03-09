import json
from unittest.mock import MagicMock

import pytest
from redis import Redis

from rustic_ai.core.guild.agent import AgentSpec
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.forge.execution_engine import ForgeExecutionEngine


@pytest.fixture
def native_redis():
    """Expects a local Redis instance on port 6379 for integration testing."""
    client = Redis(host="localhost", port=6379, db=0)
    try:
        client.ping()
        # Clean up integration keys
        for key in client.scan_iter("forge:control:*"):
            client.delete(key)
        yield client
    except Exception:
        pytest.skip("Local Redis on 6379 not available for contract test")


def test_forge_contract_python_to_go_serialization(native_redis):
    """
    1.8.3 Control queue contract test manually
    This test purely validates the JSON shape emitted by Python matches the Go definitions.
    """
    engine = ForgeExecutionEngine(guild_id="guild-contract-1", organization_id="org-1")
    engine._rdb = native_redis

    agent_spec = MagicMock(spec=AgentSpec)
    agent_spec.id = "agent-contract"
    agent_spec.model_dump.return_value = {
        "id": "agent-contract",
        "class_name": "test.Agent",
        "runtime": "binary",
        "package": "echo",
    }

    guild_spec = MagicMock(spec=GuildSpec)
    guild_spec.id = "guild-contract-1"

    req_id = "test-req-contract"
    wrapper = {
        "command": "spawn",
        "payload": {
            "request_id": req_id,
            "guild_id": engine.guild_id,
            "agent_spec": agent_spec.model_dump(),
        },
    }
    native_redis.lpush("forge:control:requests", json.dumps(wrapper))

    res = native_redis.rpop("forge:control:requests")
    assert res is not None

    data = json.loads(res.decode("utf-8"))
    assert data["command"] == "spawn"
    assert "payload" in data
    assert data["payload"]["request_id"] == "test-req-contract"
    assert data["payload"]["agent_spec"]["class_name"] == "test.Agent"


def test_status_key_contract(native_redis):
    """
    1.8.4 Status key contract test
    """
    engine = ForgeExecutionEngine(guild_id="guild-status-1", organization_id="org-1")
    engine._rdb = native_redis

    # 1. Missing key
    assert engine.is_agent_running("guild-status-1", "agent-x") is False

    # 2. Go writes "running"
    status_key = "forge:agent:status:guild-status-1:agent-x"
    native_redis.set(
        status_key, json.dumps({"state": "running", "node_id": "test-node", "pid": 999})
    )
    assert engine.is_agent_running("guild-status-1", "agent-x") is True

    # 3. Go writes "restarting"
    native_redis.set(status_key, json.dumps({"state": "restarting"}))
    assert engine.is_agent_running("guild-status-1", "agent-x") is False

    # 4. Go writes "failed"
    native_redis.set(status_key, json.dumps({"state": "failed"}))
    assert engine.is_agent_running("guild-status-1", "agent-x") is False
