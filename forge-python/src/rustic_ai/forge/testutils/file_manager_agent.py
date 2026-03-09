from typing import Optional

from pydantic import BaseModel

from rustic_ai.core.guild import agent
from rustic_ai.core.guild.agent import Agent, ProcessContext
from rustic_ai.core.guild.agent_ext.depends.filesystem import FileSystem


class WriteToFile(BaseModel):
    filename: str
    content: str
    guild_path: bool = False


class FileWriteResponse(BaseModel):
    filename: str
    result: str = "success"
    error: Optional[str] = None


class ReadFromFile(BaseModel):
    filename: str
    guild_path: bool = False


class FileReadResponse(BaseModel):
    filename: str
    content: Optional[str] = None
    result: str = "success"
    error: Optional[str] = None


class FileManagerAgent(Agent):
    """
    The File Manager Reads/Writes/Deletes files via a filesystem dependency.
    """

    @agent.processor(
        WriteToFile, depends_on=["filesystem:agent_fs", "filesystem:guild_fs:True"]
    )
    def write_file(
        self,
        ctx: ProcessContext[WriteToFile],
        agent_fs: FileSystem,
        guild_fs: FileSystem,
    ):
        data: WriteToFile = ctx.payload
        fs = guild_fs if data.guild_path else agent_fs

        try:
            with fs.open(data.filename, "wb") as f:
                f.write(data.content.encode("utf-8"))
            ctx.send(FileWriteResponse(filename=data.filename, result="success"))
        except Exception as e:
            ctx.send(
                FileWriteResponse(filename=data.filename, result="failed", error=str(e))
            )

    @agent.processor(
        ReadFromFile, depends_on=["filesystem:agent_fs", "filesystem:guild_fs:True"]
    )
    def read_file(
        self,
        ctx: ProcessContext[ReadFromFile],
        agent_fs: FileSystem,
        guild_fs: FileSystem,
    ):
        data: ReadFromFile = ctx.payload
        fs = guild_fs if data.guild_path else agent_fs

        try:
            with fs.open(data.filename, "rb") as f:
                content = f.read()
            ctx.send(
                FileReadResponse(
                    filename=data.filename,
                    content=content.decode("utf-8"),
                    result="success",
                )
            )
        except Exception as e:
            ctx.send(
                FileReadResponse(filename=data.filename, result="failed", error=str(e))
            )
