import subprocess

import pytest
import yaml
from rustic_ai.core.guild.dsl import GuildSpec, AgentSpec


def test_guild_spec_roundtrip(helper_bin, fixture_yaml_files):
    """
    Contract test 1.1.4.1:
    Python loads a GuildSpec -> Dumps to JSON -> Go parses it -> Go Dumps to JSON -> Python validates it.
    """
    for yaml_path in fixture_yaml_files:
        with open(yaml_path, "r") as f:
            data = yaml.safe_load(f)

        py_spec = GuildSpec.model_validate(data)
        payload = py_spec.model_dump_json(exclude_none=True).encode("utf-8")

        result = subprocess.run(
            [str(helper_bin), "parse-guild"], input=payload, capture_output=True
        )

        if result.returncode != 0:
            pytest.fail(
                f"Go binary failed for {yaml_path.name}: {result.stderr.decode('utf-8')}"
            )

        try:
            go_spec = GuildSpec.model_validate_json(result.stdout)
        except Exception as e:
            pytest.fail(
                f"Python failed to validate Go's output for {yaml_path.name}. "
                f"Error: {e}\nGo Output: {result.stdout.decode('utf-8')}"
            )

        py_dict = py_spec.model_dump(exclude_none=True)
        go_dict = go_spec.model_dump(exclude_none=True)
        assert py_dict == go_dict, f"Structural mismatch in {yaml_path.name}"


def test_agent_spec_roundtrip(helper_bin, fixture_yaml_files):
    """
    Contract test 1.1.4.2:
    Isolates AgentSpec roundtripping using just the agents slice from the YAML tests.
    """
    for yaml_path in fixture_yaml_files:
        with open(yaml_path, "r") as f:
            data = yaml.safe_load(f)

        if "agents" not in data:
            continue

        for agent_data in data["agents"]:
            py_spec = AgentSpec.model_validate(agent_data)
            payload = py_spec.model_dump_json(exclude_none=True).encode("utf-8")

            result = subprocess.run(
                [str(helper_bin), "parse-agent"], input=payload, capture_output=True
            )

            if result.returncode != 0:
                pytest.fail(
                    f"Go binary failed for {yaml_path.name}: {result.stderr.decode('utf-8')}"
                )

            go_spec = AgentSpec.model_validate_json(result.stdout)

            py_dict = py_spec.model_dump(exclude_none=True)
            go_dict = go_spec.model_dump(exclude_none=True)
            assert py_dict == go_dict, f"Agent Spec mismatch in {yaml_path.name}"
