package store

import "errors"

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record already exists")
)

type Store interface {
	CatalogStore

	CreateGuild(guild *GuildModel) error
	CreateGuildWithAgents(guild *GuildModel, agents []AgentModel) error
	GetGuild(id string) (*GuildModel, error)
	GetGuildByName(name string) (*GuildModel, error)
	ListGuilds() ([]GuildModel, error)
	UpdateGuildStatus(id string, status GuildStatus) error
	UpdateGuild(guild *GuildModel) error
	DeleteGuild(id string) error
	PurgeGuild(guild *GuildModel) error
	CreateGuildRelaunch(entry *GuildRelaunchModel) error

	CreateAgent(agent *AgentModel) error
	GetAgent(guildID, id string) (*AgentModel, error)
	ListAgentsByGuild(guildID string) ([]AgentModel, error)
	UpdateAgentStatus(guildID, id string, status AgentStatus) error
	UpdateAgent(agent *AgentModel) error
	DeleteAgent(guildID, id string) error
	CreateGuildRoute(route *GuildRoutes) error
	UpdateGuildRouteStatus(guildID, routeID string, status RouteStatus) error
	ProcessHeartbeatStatus(
		guildID, agentID string,
		agentStatus AgentStatus,
		guildStatus GuildStatus,
	) (effectiveAgentStatus AgentStatus, agentFound bool, err error)

	CreateBoard(board *Board) error
	GetBoard(id string) (*Board, error)
	GetBoardsByGuild(guildID string) ([]Board, error)
	AddMessageToBoard(boardID, messageID string) error
	GetBoardMessageIDs(boardID string) ([]string, error)
	RemoveMessageFromBoard(boardID, messageID string) error

	Close() error
}
