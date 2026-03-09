import json
import logging
import os
import subprocess
import tempfile
import time
from pathlib import Path
from typing import Generator

import pytest
import requests
import sys
from redis import Redis

def _find_repo_root() -> Path:
    cur = Path(__file__).resolve()
    for candidate in [cur] + list(cur.parents):
        if (candidate / "forge-go").is_dir() and (candidate / "forge-python").is_dir():
            return candidate
        nested = candidate / "rustic-go"
        if (nested / "forge-go").is_dir() and (nested / "forge-python").is_dir():
            return nested
    raise RuntimeError("unable to locate repo root containing forge-go and forge-python")


repo_root = _find_repo_root()


@pytest.fixture(scope="module")
def redis_server() -> Generator[int, None, None]:
    # Start a local redis-server
    port = 26380  # use different port to avoid clashes
    import shutil

    redis_bin = shutil.which("redis-server")
    if not redis_bin:
        pytest.skip("redis-server not found in PATH")

    proc = subprocess.Popen(
        [redis_bin, "--port", str(port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    # Wait for redis
    r = Redis(host="localhost", port=port)
    for _ in range(20):
        try:
            if r.ping():
                r.flushall()
                break
        except Exception:
            time.sleep(0.5)
    else:
        proc.kill()
        pytest.fail("Could not start redis-server")

    yield port

    proc.terminate()
    proc.wait(timeout=5)


@pytest.fixture(scope="module")
def go_server(redis_server) -> Generator[str, None, None]:
    forge_bin = repo_root / "forge-go" / "forge"

    # Always compile forge to ensure latest code is used
    subprocess.run(
        ["go", "build", "-o", str(forge_bin), "main.go"],
        cwd=str(repo_root / "forge-go"),
        check=True,
    )

    with tempfile.TemporaryDirectory() as tmpdir:
        db_path = Path(tmpdir) / "forge_server.db"
        port = 9091

        # dummy dependency config
        dep_path = Path(tmpdir) / "deps.yaml"
        dep_path.write_text("{}")

        forge_log = open(Path(tmpdir) / "forge_server.log", "w")
        forge_proc = subprocess.Popen(
            [
                str(forge_bin),
                "server",
                "--db",
                f"sqlite://{db_path}",
                "--redis",
                f"localhost:{redis_server}",
                "--listen",
                f":{port}",
                "--dependency-config",
                str(dep_path),
            ],
            stdout=forge_log,
            stderr=forge_log,
        )

        server_url = f"http://localhost:{port}"

        # Wait for the Go server to start by hitting /ping
        for _ in range(20):
            try:
                resp = requests.get(f"{server_url}/ping")
                if resp.status_code == 200:
                    break
            except Exception:
                time.sleep(0.5)
        else:
            forge_proc.kill()
            pytest.fail(
                "Could not start Go server. Log: "
                + (Path(tmpdir) / "forge_server.log").read_text()
            )

        yield server_url

        forge_proc.terminate()
        try:
            forge_proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            forge_proc.kill()
        forge_log.close()


def test_rest_api_contract_create_guild(go_server, redis_server):
    """
    Test 8.4.4 & 8.4.7:
    Hits Go Server, creates a guild, ensures the Go Server pushes the correct
    GuildManagerAgent bootstrap spawn request to Redis.
    """
    payload = {
        "organization_id": "org-contract-1",
        "spec": {
            "id": "my-guild",
            "name": "Contract Guild",
            "description": "testing 123",
            "agents": [
                {"id": "a-1", "name": "First Agent", "class_name": "test.Agent1"}
            ],
            "properties": {},
        },
    }

    # 1. API creates guild
    resp = requests.post(f"{go_server}/guilds", json=payload)
    assert resp.status_code == 201

    data = resp.json()
    assert "guild_id" in data
    assert data["status"] == "requested"

    guild_id = data["guild_id"]

    # 2. Get guild
    resp_get = requests.get(f"{go_server}/guilds/{guild_id}")
    assert resp_get.status_code == 200
    get_data = resp_get.json()
    assert get_data["name"] == "Contract Guild"
    assert get_data["status"] == "requested"
    assert get_data["organization_id"] == "org-contract-1"

    # 3. Check Control Queue for GuildManagerAgent SpawnRequest
    r = Redis(host="localhost", port=redis_server)

    # We wait just a bit for the async write if any, though it's sync in Go
    raw_req = r.rpop("forge:control:requests")
    assert raw_req is not None, "Go server failed to push spawn request"

    req = json.loads(raw_req.decode("utf-8"))
    assert "payload" in req
    payload_data = req["payload"]

    assert payload_data["guild_id"] == guild_id
    assert payload_data["agent_spec"]["id"] == "manager_agent"
    assert payload_data["agent_spec"]["name"] == "Contract Guild Manager"
    assert (
        payload_data["agent_spec"]["class_name"]
        == "rustic_ai.forge.agents.system.guild_manager_agent.GuildManagerAgent"
    )

    assert "client_properties" in payload_data
    client_props = payload_data["client_properties"]
    assert client_props["organization_id"] == "org-contract-1"
    assert "guild_spec" in client_props


def test_rest_api_filesystem_contract(go_server):
    """
    Test File System Endpoints
    """
    import io

    # 1. Upload File
    files = {
        "file": ("test_file.txt", io.BytesIO(b"Hello Integration Tests!"), "text/plain")
    }

    # To use a known guild_id, we create one first
    payload = {
        "organization_id": "org-contract-1",
        "spec": {"name": "File Guild", "agents": []},
    }
    resp = requests.post(f"{go_server}/guilds", json=payload)
    guild_id = resp.json()["guild_id"]

    resp_up = requests.post(f"{go_server}/guilds/{guild_id}/files/", files=files)
    assert resp_up.status_code == 201

    # 2. List Files
    resp_list = requests.get(f"{go_server}/guilds/{guild_id}/files/")
    assert resp_list.status_code == 200
    file_list = resp_list.json()
    assert len(file_list) == 1
    assert file_list[0]["filename"] == "test_file.txt"
    assert file_list[0]["content_length"] == len("Hello Integration Tests!")

    # 3. Download File
    resp_dl = requests.get(f"{go_server}/guilds/{guild_id}/files/test_file.txt")
    assert resp_dl.status_code == 200
    assert resp_dl.content == b"Hello Integration Tests!"

    # 4. Delete File
    resp_del = requests.delete(f"{go_server}/guilds/{guild_id}/files/test_file.txt")
    assert resp_del.status_code == 200

    resp_list2 = requests.get(f"{go_server}/guilds/{guild_id}/files/")
    assert len(resp_list2.json()) == 0
