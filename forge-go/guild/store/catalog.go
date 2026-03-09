package store

import (
	"errors"

	"gorm.io/gorm"
)

type CatalogStore interface {
	CreateBlueprint(req *Blueprint) (*Blueprint, error)
	ListBlueprints() ([]Blueprint, error)
	GetBlueprint(id string) (*Blueprint, error)
	GetAccessibleBlueprints(userID string, orgID *string) ([]Blueprint, error)
	GetBlueprintsByTag(tag string) ([]Blueprint, error)
	GetBlueprintsByCategoryName(categoryName string) ([]Blueprint, error)
	GetBlueprintsByAuthor(authorID string) ([]Blueprint, error)
	GetBlueprintsByOrganization(orgID string) ([]Blueprint, error)
	GetBlueprintsSharedWithOrganization(orgID string) ([]Blueprint, error)
	GetOrganizationsWithSharedBlueprint(blueprintID string) ([]string, error)
	GetTags() ([]string, error)
	GetBlueprintForGuild(guildID string) (*Blueprint, error)
	AddGuildToBlueprint(blueprintID string, guildID string) error
	IsBlueprintSharedWithOrg(blueprintID string, orgID string) (bool, error)

	CreateCategory(category *BlueprintCategory) (*BlueprintCategory, error)
	GetCategory(id string) (*BlueprintCategory, error)
	ListCategories() ([]BlueprintCategory, error)

	CreateOrGetTags(tagNames []string) ([]Tag, error)

	ShareBlueprint(blueprintID string, orgID string) error
	UnshareBlueprint(blueprintID string, orgID string) error

	CreateBlueprintReview(review *BlueprintReview) (*BlueprintReview, error)
	GetBlueprintReviews(blueprintID string) ([]BlueprintReview, error)
	GetBlueprintReview(reviewID string) (*BlueprintReview, error)

	UpsertBlueprintAgentIcons(blueprintID string, icons []BlueprintAgentIcon) error
	GetBlueprintAgentIcons(blueprintID string) ([]BlueprintAgentIcon, error)
	UpsertBlueprintAgentIcon(icon *BlueprintAgentIcon) error
	GetBlueprintAgentIcon(blueprintID, agentName string) (*BlueprintAgentIcon, error)

	RegisterAgent(agent *CatalogAgentEntry) error
	GetAgentByClassName(className string) (*CatalogAgentEntry, error)
	GetAgents() ([]CatalogAgentEntry, error)
	GetAgentMessageSchema(messageFormat string) (map[string]interface{}, error)
	AddUserToGuild(guildID string, userID string) error
	RemoveUserFromGuild(guildID string, userID string) error
	GetUsersForGuild(guildID string) ([]string, error)
	GetGuildsForUser(userID string, orgID *string, statuses []string) ([]GuildModel, error)
	GetGuildsForOrg(orgID string, statuses []string) ([]GuildModel, error)
}

var _ CatalogStore = (*gormStore)(nil)

func (s *gormStore) CreateBlueprint(bp *Blueprint) (*Blueprint, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(bp).Error; err != nil {
			return err
		}

		// Link agents from spec to blueprint_agent_link
		agents, ok := bp.Spec["agents"].([]interface{})
		if !ok {
			return nil
		}
		seen := map[string]bool{}
		for _, raw := range agents {
			agentMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			className, _ := agentMap["class_name"].(string)
			if className == "" || seen[className] {
				continue
			}
			seen[className] = true
			var entry CatalogAgentEntry
			if err := tx.Where("qualified_class_name = ?", className).First(&entry).Error; err != nil {
				continue // skip agents not in catalog
			}
			link := BlueprintAgentLink{
				BlueprintID:        bp.ID,
				QualifiedClassName: className,
			}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bp, nil
}

func (s *gormStore) ListBlueprints() ([]Blueprint, error) {
	var blueprints []Blueprint
	err := s.db.Preload("Tags").Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) GetBlueprint(id string) (*Blueprint, error) {
	var bp Blueprint
	err := s.db.Preload("Tags").Preload("Commands").Preload("StarterPrompts").Preload("Reviews").Where("id = ?", id).First(&bp).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &bp, nil
}

func (s *gormStore) GetAccessibleBlueprints(userID string, orgID *string) ([]Blueprint, error) {
	var blueprints []Blueprint

	query := s.db.Model(&Blueprint{}).
		Joins("LEFT JOIN blueprintsharedwithorganization shared ON blueprint.id = shared.blueprint_id").
		Where("blueprint.exposure = ?", ExposurePublic).
		Or("blueprint.exposure = ? AND blueprint.author_id = ?", ExposurePrivate, userID)

	if orgID != nil && *orgID != "" {
		query = query.Or("blueprint.exposure = ? AND blueprint.organization_id = ?", ExposureOrganization, *orgID).
			Or("blueprint.exposure = ? AND shared.organization_id = ?", ExposureShared, *orgID)
	}

	err := query.Group("blueprint.id").Preload("Tags").Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) CreateCategory(category *BlueprintCategory) (*BlueprintCategory, error) {
	err := s.db.Create(category).Error
	return category, err
}

func (s *gormStore) GetCategory(id string) (*BlueprintCategory, error) {
	var cat BlueprintCategory
	err := s.db.Where("id = ?", id).First(&cat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &cat, err
}

func (s *gormStore) ListCategories() ([]BlueprintCategory, error) {
	var categories []BlueprintCategory
	err := s.db.Find(&categories).Error
	return categories, err
}

func (s *gormStore) CreateOrGetTags(tagNames []string) ([]Tag, error) {
	if len(tagNames) == 0 {
		return nil, nil
	}

	var tags []Tag
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, name := range tagNames {
			var tag Tag
			if err := tx.Where(Tag{Tag: name}).FirstOrCreate(&tag).Error; err != nil {
				return err
			}
			tags = append(tags, tag)
		}
		return nil
	})

	return tags, err
}

func (s *gormStore) ShareBlueprint(blueprintID string, orgID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var bp Blueprint
		if err := tx.Where("id = ?", blueprintID).First(&bp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}

		// If currently organization-scoped, auto-share with the owning org before transitioning
		if bp.Exposure == ExposureOrganization && bp.OrganizationID != nil {
			origOrgShare := BlueprintSharedWithOrganization{
				BlueprintID:    blueprintID,
				OrganizationID: *bp.OrganizationID,
			}
			if err := tx.Save(&origOrgShare).Error; err != nil {
				return err
			}
		}

		// Transition exposure to shared
		if err := tx.Model(&bp).Update("exposure", ExposureShared).Error; err != nil {
			return err
		}

		// Add the new org share
		shared := BlueprintSharedWithOrganization{
			BlueprintID:    blueprintID,
			OrganizationID: orgID,
		}
		return tx.Save(&shared).Error
	})
}

func (s *gormStore) UnshareBlueprint(blueprintID string, orgID string) error {
	if _, err := s.GetBlueprint(blueprintID); err != nil {
		return err
	}
	result := s.db.Where("blueprint_id = ? AND organization_id = ?", blueprintID, orgID).
		Delete(&BlueprintSharedWithOrganization{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *gormStore) CreateBlueprintReview(review *BlueprintReview) (*BlueprintReview, error) {
	err := s.db.Create(review).Error
	return review, err
}

func (s *gormStore) GetBlueprintReviews(blueprintID string) ([]BlueprintReview, error) {
	var reviews []BlueprintReview
	err := s.db.Where("blueprint_id = ?", blueprintID).Find(&reviews).Error
	return reviews, err
}

func (s *gormStore) GetBlueprintReview(reviewID string) (*BlueprintReview, error) {
	var review BlueprintReview
	err := s.db.Where("id = ?", reviewID).First(&review).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &review, nil
}

func (s *gormStore) UpsertBlueprintAgentIcons(blueprintID string, icons []BlueprintAgentIcon) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i := range icons {
			icons[i].BlueprintID = blueprintID
			var existing BlueprintAgentIcon
			err := tx.Where("blueprint_id = ? AND agent_name = ?", blueprintID, icons[i].AgentName).First(&existing).Error
			if err == nil {
				if err := tx.Model(&existing).Update("icon", icons[i].Icon).Error; err != nil {
					return err
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&icons[i]).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		return nil
	})
}

func (s *gormStore) GetBlueprintAgentIcons(blueprintID string) ([]BlueprintAgentIcon, error) {
	var icons []BlueprintAgentIcon
	if err := s.db.Where("blueprint_id = ?", blueprintID).Find(&icons).Error; err != nil {
		return nil, err
	}

	// Build set of agent names already covered
	covered := map[string]bool{}
	for _, icon := range icons {
		covered[icon.AgentName] = true
	}

	// Fallback to default agent_icon table for agents not already covered
	bp, err := s.GetBlueprint(blueprintID)
	if err != nil {
		return icons, nil // return what we have if blueprint lookup fails
	}

	agents, ok := bp.Spec["agents"].([]interface{})
	if !ok {
		return icons, nil
	}

	var classNames []string
	agentClassToName := map[string]string{}
	for _, raw := range agents {
		agentMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := agentMap["name"].(string)
		className, _ := agentMap["class_name"].(string)
		if name != "" && className != "" && !covered[name] {
			classNames = append(classNames, className)
			agentClassToName[className] = name
		}
	}

	if len(classNames) > 0 {
		var defaults []AgentIcon
		if err := s.db.Where("agent_class IN ?", classNames).Find(&defaults).Error; err == nil {
			for _, d := range defaults {
				if agentName, ok := agentClassToName[d.AgentClass]; ok {
					icons = append(icons, BlueprintAgentIcon{
						BlueprintID: blueprintID,
						AgentName:   agentName,
						Icon:        d.Icon,
					})
				}
			}
		}
	}

	return icons, nil
}

func (s *gormStore) UpsertBlueprintAgentIcon(icon *BlueprintAgentIcon) error {
	var existing BlueprintAgentIcon
	err := s.db.Where("blueprint_id = ? AND agent_name = ?", icon.BlueprintID, icon.AgentName).First(&existing).Error
	if err == nil {
		return s.db.Model(&existing).Update("icon", icon.Icon).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.db.Create(icon).Error
	}
	return err
}

func (s *gormStore) GetBlueprintAgentIcon(blueprintID, agentName string) (*BlueprintAgentIcon, error) {
	var icon BlueprintAgentIcon
	err := s.db.Where("blueprint_id = ? AND agent_name = ?", blueprintID, agentName).First(&icon).Error
	if err == nil {
		return &icon, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Fallback: look up agent's class_name from blueprint spec, then check agent_icon defaults
	bp, err := s.GetBlueprint(blueprintID)
	if err != nil {
		return nil, ErrNotFound
	}
	agents, ok := bp.Spec["agents"].([]interface{})
	if !ok {
		return nil, ErrNotFound
	}
	var className string
	for _, raw := range agents {
		agentMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := agentMap["name"].(string); name == agentName {
			className, _ = agentMap["class_name"].(string)
			break
		}
	}
	if className == "" {
		return nil, ErrNotFound
	}

	var defaultIcon AgentIcon
	err = s.db.Where("agent_class = ?", className).First(&defaultIcon).Error
	if err != nil {
		return nil, ErrNotFound
	}
	return &BlueprintAgentIcon{
		BlueprintID: blueprintID,
		AgentName:   agentName,
		Icon:        defaultIcon.Icon,
	}, nil
}

func (s *gormStore) RegisterAgent(agent *CatalogAgentEntry) error {
	existing, err := s.GetAgentByClassName(agent.QualifiedClassName)
	if err != nil && err != ErrNotFound {
		return err
	}
	if existing != nil {
		return gorm.ErrDuplicatedKey
	}
	return s.db.Create(agent).Error
}

func (s *gormStore) GetAgents() ([]CatalogAgentEntry, error) {
	var agents []CatalogAgentEntry
	err := s.db.Find(&agents).Error
	return agents, err
}

func (s *gormStore) GetBlueprintsByTag(tag string) ([]Blueprint, error) {
	var blueprints []Blueprint
	err := s.db.Model(&Blueprint{}).
		Joins("JOIN blueprint_tag bt ON bt.blueprint_id = blueprint.id").
		Joins("JOIN tag t ON t.id = bt.tag_id").
		Where("t.tag = ?", tag).
		Preload("Tags").
		Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) GetBlueprintsByCategoryName(categoryName string) ([]Blueprint, error) {
	var category BlueprintCategory
	if err := s.db.Where("name = ?", categoryName).First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var blueprints []Blueprint
	if err := s.db.Where("category_id = ?", category.ID).Preload("Tags").Find(&blueprints).Error; err != nil {
		return nil, err
	}
	return blueprints, nil
}

func (s *gormStore) GetBlueprintsByAuthor(authorID string) ([]Blueprint, error) {
	var blueprints []Blueprint
	err := s.db.Where("author_id = ?", authorID).Preload("Tags").Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) GetBlueprintsByOrganization(orgID string) ([]Blueprint, error) {
	var blueprints []Blueprint
	err := s.db.Where("organization_id = ?", orgID).Preload("Tags").Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) GetBlueprintsSharedWithOrganization(orgID string) ([]Blueprint, error) {
	var blueprints []Blueprint
	err := s.db.Model(&Blueprint{}).
		Joins("JOIN blueprintsharedwithorganization bso ON bso.blueprint_id = blueprint.id").
		Where("bso.organization_id = ?", orgID).
		Preload("Tags").
		Find(&blueprints).Error
	return blueprints, err
}

func (s *gormStore) GetOrganizationsWithSharedBlueprint(blueprintID string) ([]string, error) {
	var orgIDs []string
	err := s.db.Model(&BlueprintSharedWithOrganization{}).
		Where("blueprint_id = ?", blueprintID).
		Pluck("organization_id", &orgIDs).Error
	return orgIDs, err
}

func (s *gormStore) GetTags() ([]string, error) {
	var tags []string
	err := s.db.Model(&Tag{}).Distinct("tag").Pluck("tag", &tags).Error
	return tags, err
}

func (s *gormStore) AddGuildToBlueprint(blueprintID string, guildID string) error {
	if _, err := s.GetBlueprint(blueprintID); err != nil {
		return err
	}
	if _, err := s.GetGuild(guildID); err != nil {
		return err
	}
	var existing BlueprintGuild
	if err := s.db.Where("guild_id = ?", guildID).First(&existing).Error; err == nil {
		return gorm.ErrDuplicatedKey
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(&BlueprintGuild{GuildID: guildID, BlueprintID: blueprintID}).Error
}

func (s *gormStore) GetBlueprintForGuild(guildID string) (*Blueprint, error) {
	if _, err := s.GetGuild(guildID); err != nil {
		return nil, err
	}
	var bg BlueprintGuild
	if err := s.db.Where("guild_id = ?", guildID).First(&bg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.GetBlueprint(bg.BlueprintID)
}

func (s *gormStore) IsBlueprintSharedWithOrg(blueprintID string, orgID string) (bool, error) {
	var n int64
	err := s.db.Model(&BlueprintSharedWithOrganization{}).
		Where("blueprint_id = ? AND organization_id = ?", blueprintID, orgID).
		Count(&n).Error
	return n > 0, err
}

func (s *gormStore) GetAgentByClassName(className string) (*CatalogAgentEntry, error) {
	var agent CatalogAgentEntry
	err := s.db.Where("qualified_class_name = ?", className).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &agent, nil
}

func (s *gormStore) GetAgentMessageSchema(messageFormat string) (map[string]interface{}, error) {
	var agents []CatalogAgentEntry
	if err := s.db.Find(&agents).Error; err != nil {
		return nil, err
	}
	for _, agent := range agents {
		handlers := map[string]interface{}(agent.MessageHandlers)
		if len(handlers) == 0 {
			continue
		}
		for _, raw := range handlers {
			handler, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if mf, _ := handler["message_format"].(string); mf == messageFormat {
				if schema, ok := handler["message_format_schema"].(map[string]interface{}); ok {
					return schema, nil
				}
				return nil, nil
			}
		}
	}
	return nil, ErrNotFound
}

func (s *gormStore) AddUserToGuild(guildID string, userID string) error {
	if _, err := s.GetGuild(guildID); err != nil {
		return err
	}
	var existing UserGuild
	if err := s.db.Where("guild_id = ? AND user_id = ?", guildID, userID).First(&existing).Error; err == nil {
		return gorm.ErrDuplicatedKey
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(&UserGuild{GuildID: guildID, UserID: userID}).Error
}

func (s *gormStore) RemoveUserFromGuild(guildID string, userID string) error {
	if _, err := s.GetGuild(guildID); err != nil {
		return err
	}
	res := s.db.Where("guild_id = ? AND user_id = ?", guildID, userID).Delete(&UserGuild{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *gormStore) GetUsersForGuild(guildID string) ([]string, error) {
	if _, err := s.GetGuild(guildID); err != nil {
		return nil, err
	}
	var ids []string
	err := s.db.Model(&UserGuild{}).Where("guild_id = ?", guildID).Pluck("user_id", &ids).Error
	return ids, err
}

func (s *gormStore) GetGuildsForUser(userID string, orgID *string, statuses []string) ([]GuildModel, error) {
	var guilds []GuildModel
	q := s.db.Model(&GuildModel{}).
		Joins("JOIN user_guild ug ON ug.guild_id = guilds.id").
		Where("ug.user_id = ?", userID)
	if orgID != nil && *orgID != "" {
		q = q.Where("guilds.organization_id = ?", *orgID)
	}
	if len(statuses) > 0 {
		q = q.Where("guilds.status IN ?", statuses)
	}
	err := q.Find(&guilds).Error
	return guilds, err
}

func (s *gormStore) GetGuildsForOrg(orgID string, statuses []string) ([]GuildModel, error) {
	var guilds []GuildModel
	q := s.db.Where("organization_id = ?", orgID)
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	err := q.Find(&guilds).Error
	return guilds, err
}
