import logging
import signal
import threading
import time
from typing import Any, Dict, Optional, Type

from rustic_ai.core.guild.agent import AgentSpec
from rustic_ai.core.guild.dsl import GuildSpec
from rustic_ai.core.messaging import Client, MessageTrackingClient, MessagingConfig
from rustic_ai.core.guild.execution.agent_wrapper import AgentWrapper

logger = logging.getLogger(__name__)


class ForgeAgentWrapper(AgentWrapper):
    """
    An implementation of AgentWrapper that runs as a standalone Python process
    managed by the Go-based Forge AgentSupervisor.
    """

    def __init__(
        self,
        guild_spec: GuildSpec,
        agent_spec: AgentSpec,
        messaging_config: MessagingConfig,
        machine_id: int,
        client_type: Type[Client] = MessageTrackingClient,
        client_properties: Optional[Dict[str, Any]] = None,
        organization_id: Optional[str] = None,
    ):
        if client_properties is None:
            client_properties = {}

        super().__init__(
            guild_spec=guild_spec,
            agent_spec=agent_spec,
            messaging_config=messaging_config,
            machine_id=machine_id,
            client_type=client_type,
            client_properties=client_properties,
            organization_id=organization_id,
        )

        self.shutdown_event = threading.Event()
        self.is_running = False

    def run(self) -> None:
        """Initializes the agent and blocks until shutdown is signaled."""
        try:
            self.is_running = True
            logger.info(f"Initializing agent {self.agent_spec.name} in Forge wrapper")
            self.initialize_agent()
            logger.info(f"Agent {self.agent_spec.name} initialized successfully.")

            def signal_handler(signum: int, frame: Any) -> None:
                logger.info(
                    f"Received signal {signum}, initiating shutdown for {self.agent_spec.id}..."
                )
                self.shutdown_event.set()

            signal.signal(signal.SIGINT, signal_handler)
            signal.signal(signal.SIGTERM, signal_handler)

            while not self.shutdown_event.is_set():
                time.sleep(0.5)

            logger.info(f"Agent {self.agent_spec.id} shutting down cleanly.")
            self.shutdown()

        except Exception as e:
            logger.error(
                f"Error running agent {self.agent_spec.name} in Forge wrapper: {e}",
                exc_info=True,
            )
            self.is_running = False
            raise

    def shutdown(self) -> None:
        self.shutdown_event.set()
        super().shutdown()
        self.is_running = False
