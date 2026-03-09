import json
import os
import urllib.request
from typing import Any, Dict

from rustic_ai.core.agents.agent import Agent
from rustic_ai.core.guild.dsl import BaseAgentProps
from rustic_ai.core.handlers import processor


class MockLLMAgentProps(BaseAgentProps):
    base_url: str
    model: str
    custom_llm_provider: str = ""


class MockLLMAgent(Agent[MockLLMAgentProps]):
    """
    A lightweight mock LLM Agent for Go integration tests.
    It verifies that OPENAI_API_KEY is injected by the Go supervisor,
    and forwards the prompt to the Go httptest server.
    """

    @processor(dict)
    def invoke_mock_llm(self, ctx) -> Dict[str, Any]:
        api_key = os.environ.get("OPENAI_API_KEY")
        if not api_key:
            raise ValueError(
                "OPENAI_API_KEY secret was not injected by Forge Go AgentSupervisor"
            )

        url = f"{self.config.base_url.rstrip('/')}/chat/completions"

        req = urllib.request.Request(
            url,
            data=json.dumps(ctx.payload).encode("utf-8"),
            headers={
                "Content-Type": "application/json",
                "Authorization": f"Bearer {api_key}",
            },
            method="POST",
        )

        with urllib.request.urlopen(req) as response:
            result = json.loads(response.read().decode("utf-8"))

        return result
