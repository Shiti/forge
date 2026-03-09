package store

import (
	"time"

	"github.com/rustic-ai/forge/forge-go/helper/idgen"
	"gorm.io/gorm"
)

type Board struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"index" json:"name"`
	GuildID   string    `gorm:"index" json:"guild_id"`
	CreatedBy string    `gorm:"index" json:"created_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	IsDefault bool      `gorm:"default:false" json:"is_default"`
	IsPrivate bool      `gorm:"default:false" json:"is_private"`
}

func (Board) TableName() string { return "board" }

func (b *Board) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = idgen.NewShortUUID()
	}
	return nil
}

type BoardMessage struct {
	BoardID   string `gorm:"primaryKey" json:"board_id"`
	MessageID string `gorm:"primaryKey" json:"message_id"`
}

func (BoardMessage) TableName() string { return "board_message" }
