package store

import (
	"time"

	"github.com/rustic-ai/forge/forge-go/helper/idgen"
	"gorm.io/gorm"
)

type BlueprintExposure string

const (
	ExposurePublic                  BlueprintExposure = "public"
	ExposurePrivate                 BlueprintExposure = "private"
	ExposureOrganization            BlueprintExposure = "organization"
	ExposureShared                  BlueprintExposure = "shared"
	ExposureOrganizationAndChildren BlueprintExposure = "organization_and_children"
)

type Blueprint struct {
	ID             string            `gorm:"primaryKey" json:"id"`
	Name           string            `gorm:"index" json:"name"`
	Description    string            `gorm:"index" json:"description"`
	Version        string            `json:"version"`
	Icon           *string           `json:"icon"`
	IntroMsg       *string           `json:"intro_msg"`
	Exposure       BlueprintExposure `gorm:"default:'private'" json:"exposure"`
	AuthorID       string            `json:"author_id"`
	OrganizationID *string           `json:"organization_id"`
	CategoryID     *string           `gorm:"index" json:"category_id"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Spec JSONB `gorm:"type:jsonb" json:"spec"`

	// Relationships
	Category       *BlueprintCategory       `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	Tags           []Tag                    `gorm:"many2many:blueprint_tag;" json:"tags,omitempty"`
	Commands       []BlueprintCommand       `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"commands,omitempty"`
	StarterPrompts []BlueprintStarterPrompt `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"starter_prompts,omitempty"`
	Reviews        []BlueprintReview        `gorm:"foreignKey:BlueprintID;constraint:OnDelete:CASCADE" json:"reviews,omitempty"`
	Agents         []CatalogAgentEntry      `gorm:"many2many:blueprint_agent_link;joinForeignKey:BlueprintID;joinReferences:QualifiedClassName;" json:"agents,omitempty"`
}

func (Blueprint) TableName() string {
	return "blueprint"
}

func (b *Blueprint) normalizeDefaults() {
	ensureJSONB(&b.Spec)
	if b.Tags == nil {
		b.Tags = []Tag{}
	}
	if b.Commands == nil {
		b.Commands = []BlueprintCommand{}
	}
	if b.StarterPrompts == nil {
		b.StarterPrompts = []BlueprintStarterPrompt{}
	}
	if b.Reviews == nil {
		b.Reviews = []BlueprintReview{}
	}
	if b.Agents == nil {
		b.Agents = []CatalogAgentEntry{}
	}
}

func (b *Blueprint) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	b.normalizeDefaults()
	return nil
}

func (b *Blueprint) AfterFind(tx *gorm.DB) (err error) {
	b.normalizeDefaults()
	return nil
}

type BlueprintSharedWithOrganization struct {
	OrganizationID string `gorm:"primaryKey" json:"organization_id"`
	BlueprintID    string `gorm:"primaryKey;index" json:"blueprint_id"`
}

func (BlueprintSharedWithOrganization) TableName() string {
	return "blueprintsharedwithorganization"
}

type BlueprintCommand struct {
	ID          string `gorm:"primaryKey" json:"id"`
	BlueprintID string `gorm:"index" json:"blueprint_id"`
	Command     string `json:"command"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"blueprint,omitempty"`
}

func (BlueprintCommand) TableName() string {
	return "blueprint_command"
}

func (b *BlueprintCommand) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	return nil
}

type BlueprintStarterPrompt struct {
	ID          string `gorm:"primaryKey" json:"id"`
	BlueprintID string `gorm:"index" json:"blueprint_id"`
	Prompt      string `json:"prompt"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"blueprint,omitempty"`
}

func (BlueprintStarterPrompt) TableName() string {
	return "blueprint_starter_prompt"
}

func (b *BlueprintStarterPrompt) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	return nil
}

type Tag struct {
	ID        int       `gorm:"primaryKey;autoIncrement" json:"id"`
	Tag       string    `gorm:"index;unique" json:"tag"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Blueprints []Blueprint `gorm:"many2many:blueprint_tag;" json:"blueprints,omitempty"`
}

func (Tag) TableName() string {
	return "tag"
}

// Explicit join table model to match SQLModel
type BlueprintTag struct {
	TagID       int    `gorm:"primaryKey" json:"tag_id"`
	BlueprintID string `gorm:"primaryKey" json:"blueprint_id"`
}

func (BlueprintTag) TableName() string {
	return "blueprint_tag"
}

type BlueprintCategory struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Blueprints []Blueprint `gorm:"foreignKey:CategoryID" json:"blueprints,omitempty"`
}

func (BlueprintCategory) TableName() string {
	return "blueprint_category"
}

func (b *BlueprintCategory) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	return nil
}

type BlueprintReview struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	BlueprintID string    `gorm:"index" json:"blueprint_id"`
	UserID      string    `gorm:"index" json:"user_id"`
	Rating      int       `json:"rating"`
	Review      *string   `json:"review"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Blueprint *Blueprint `gorm:"foreignKey:BlueprintID" json:"blueprint,omitempty"`
}

func (BlueprintReview) TableName() string {
	return "blueprint_reviews"
}

func (b *BlueprintReview) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	return nil
}

type CatalogAgentEntry struct {
	QualifiedClassName string  `gorm:"primaryKey" json:"qualified_class_name"`
	AgentName          string  `json:"agent_name"`
	AgentDoc           *string `json:"agent_doc"`
	AgentPropsSchema   JSONB   `gorm:"type:jsonb" json:"agent_props_schema"`
	MessageHandlers    JSONB   `gorm:"type:jsonb" json:"message_handlers"`
	AgentDependencies  JSONB   `gorm:"type:jsonb" json:"agent_dependencies"`

	// Back-reference to blueprints via many2many join table
	Blueprints []Blueprint `gorm:"many2many:blueprint_agent_link;joinForeignKey:QualifiedClassName;joinReferences:BlueprintID;" json:"blueprints,omitempty"`
}

func (CatalogAgentEntry) TableName() string {
	return "agent_entry"
}

func (e *CatalogAgentEntry) normalizeDefaults() {
	if e.AgentDoc == nil {
		empty := ""
		e.AgentDoc = &empty
	}
	ensureJSONB(&e.AgentPropsSchema)
	ensureJSONB(&e.MessageHandlers)
	ensureJSONB(&e.AgentDependencies)
}

func (e *CatalogAgentEntry) BeforeCreate(tx *gorm.DB) (err error) {
	e.normalizeDefaults()
	return nil
}

func (e *CatalogAgentEntry) AfterFind(tx *gorm.DB) (err error) {
	e.normalizeDefaults()
	return nil
}

type BlueprintAgentLink struct {
	BlueprintID        string `gorm:"primaryKey;index" json:"blueprint_id"`
	QualifiedClassName string `gorm:"primaryKey;index" json:"qualified_class_name"`
}

func (BlueprintAgentLink) TableName() string {
	return "blueprint_agent_link"
}

type AgentIcon struct {
	AgentClass string `gorm:"primaryKey;index" json:"agent_class"`
	Icon       string `json:"icon"`
}

func (AgentIcon) TableName() string {
	return "agent_icon"
}

type BlueprintAgentIcon struct {
	BlueprintID string `gorm:"primaryKey;index" json:"blueprint_id"`
	AgentName   string `gorm:"primaryKey;index" json:"agent_name"`
	Icon        string `json:"icon"`
}

func (BlueprintAgentIcon) TableName() string {
	return "blueprint_agent_icon"
}

type BlueprintGuild struct {
	GuildID     string `gorm:"primaryKey;index" json:"guild_id"`
	BlueprintID string `gorm:"index" json:"blueprint_id"`
}

func (BlueprintGuild) TableName() string {
	return "blueprint_guild"
}

type UserGuild struct {
	GuildID string `gorm:"primaryKey;index" json:"guild_id"`
	UserID  string `gorm:"primaryKey;index" json:"user_id"`
}

func (UserGuild) TableName() string {
	return "user_guild"
}
