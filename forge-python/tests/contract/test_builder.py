import subprocess

import pytest
import yaml
from rustic_ai.core.guild.dsl import GuildSpec


def test_builder_contract(helper_bin, fixture_yaml_files):
    """
    Contract test 1.1.4.3:
    Verifies that defaults and configurations applied by Go's GuildBuilder
    result in a structurally valid GuildSpec that Python can ingest without loss.
    """
    for yaml_path in fixture_yaml_files:
        with open(yaml_path, "r") as f:
            data = yaml.safe_load(f)

        py_spec = GuildSpec.model_validate(data)
        payload = py_spec.model_dump_json(exclude_none=True).encode("utf-8")

        result = subprocess.run(
            [str(helper_bin), "build-guild"], input=payload, capture_output=True
        )

        if result.returncode != 0:
            pytest.fail(
                f"Go binary failed to build-guild for {yaml_path.name}: "
                f"{result.stderr.decode('utf-8')}"
            )

        try:
            go_spec = GuildSpec.model_validate_json(result.stdout)
        except Exception as e:
            pytest.fail(
                f"Python failed to validate Go's built output for {yaml_path.name}. "
                f"Error: {e}\nGo Output: {result.stdout.decode('utf-8')}"
            )

        py_dict = py_spec.model_dump(exclude_none=True)
        go_dict = go_spec.model_dump(exclude_none=True)

        assert len(py_dict.get("agents", [])) == len(go_dict.get("agents", []))
