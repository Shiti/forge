import subprocess

import pytest
import yaml
from sqlmodel import Session, create_engine, select
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.core.guild.metastore.models import GuildModel, AgentModel, GuildRoutes


def test_metastore_contract(helper_bin, fixture_yaml_files, tmp_path):
    """
    Contract test 1.1.4.4:
    Go creates a SQLite db from the GuildSpec using GORM.
    Python reads the SQL DB using SQLModel and asserts it matches the parsed original spec.
    """
    for yaml_path in fixture_yaml_files:
        with open(yaml_path, "r") as f:
            data = yaml.safe_load(f)

        py_spec = GuildSpec.model_validate(data)
        payload = py_spec.model_dump_json(exclude_none=True).encode("utf-8")

        db_path = tmp_path / f"{yaml_path.stem}.db"

        result = subprocess.run(
            [str(helper_bin), "metastore-write", str(db_path)],
            input=payload,
            capture_output=True,
        )

        if result.returncode != 0:
            pytest.fail(
                f"Go binary failed metastore-write for {yaml_path.name}: "
                f"{result.stderr.decode('utf-8')}"
            )

        assert db_path.exists(), f"SQLite database was not created at {db_path}"

        engine = create_engine(f"sqlite:///{db_path}")

        with Session(engine) as session:
            guilds = session.exec(select(GuildModel)).unique().all()
            assert len(guilds) == 1, f"Expected 1 guild, found {len(guilds)}"
            assert guilds[0].id == py_spec.id
            assert guilds[0].name == py_spec.name

            agents = session.exec(select(AgentModel)).unique().all()
            assert len(agents) == len(py_spec.agents)

            db_agent_names = {a.name for a in agents}
            py_agent_names = {a.name for a in py_spec.agents}
            assert db_agent_names == py_agent_names

            routes = session.exec(select(GuildRoutes)).unique().all()
            expected_routes = len(py_spec.routes.steps) if py_spec.routes else 0
            assert len(routes) == expected_routes
