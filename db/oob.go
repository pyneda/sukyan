package db

import (
	"github.com/rs/zerolog/log"
	"strings"
	"time"
)

// type OOBSession struct {
// 	gorm.Model
// }

type OOBTest struct {
	BaseModel
	Code              IssueCode `json:"code"`
	TestName          string    `json:"test_name"`
	Target            string    `json:"target"`
	HistoryID         *uint     `json:"history_id"`
	HistoryItem       *History  `gorm:"foreignKey:HistoryID" json:"-"`
	InteractionDomain string    `gorm:"index" json:"interaction_domain"`
	InteractionFullID string    `gorm:"index" json:"interaction_id"`
	Payload           string    `json:"payload"`
	InsertionPoint    string    `json:"insertion_point"`
	Workspace         Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID       *uint     `json:"workspace_id"`
}

// CreateOOBTest saves an OOBTest to the database
func (d *DatabaseConnection) CreateOOBTest(item OOBTest) (OOBTest, error) {
	item.InteractionFullID = strings.ToLower(item.InteractionFullID)
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("item", item).Msg("Failed to create OOBTest")
	}
	return item, result.Error
}

type OOBInteraction struct {
	BaseModel
	OOBTestID *uint   `json:"oob_test_id"`
	OOBTest   OOBTest `json:"oob_test" gorm:"foreignKey:OOBTestID"`

	Protocol      string    `json:"protocol"`
	FullID        string    `json:"full_id"`
	UniqueID      string    `json:"unique_id"`
	QType         string    `json:"qtype"`
	RawRequest    string    `json:"raw_request"`
	RawResponse   string    `json:"raw_response"`
	RemoteAddress string    `json:"remote_address"`
	Timestamp     time.Time `json:"timestamp"`
	Workspace     Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID   *uint     `json:"workspace_id"`
	IssueID       *uint     `json:"issue_id"`
}

// CreateInteraction saves an issue to the database
func (d *DatabaseConnection) CreateInteraction(item *OOBInteraction) (*OOBInteraction, error) {
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("interaction", item).Msg("Failed to create interaction")
	}
	return item, result.Error
}

// GetInteraction fetches an OOBInteraction by its ID, including its associated OOBTest.
func (d *DatabaseConnection) GetInteraction(interactionID uint) (*OOBInteraction, error) {
	var interaction OOBInteraction
	result := d.db.Preload("OOBTest").First(&interaction, interactionID)
	if result.Error != nil {
		log.Error().Uint("interactionID", interactionID).Err(result.Error).Msg("Failed to fetch interaction")
		return nil, result.Error
	}
	return &interaction, nil
}

func (d *DatabaseConnection) MatchInteractionWithOOBTest(interaction OOBInteraction) (OOBTest, error) {
	oobTest := OOBTest{}
	fullID := strings.ToLower(interaction.FullID)
	result := d.db.Where(&OOBTest{InteractionFullID: fullID}).First(&oobTest)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("interaction", interaction).Msg("Failed to find OOBTest")
	} else {
		log.Info().Interface("oobTest", oobTest).Interface("interaction", interaction).Msg("Matched Interaction and OOBTest")
		interaction.OOBTestID = &oobTest.ID
		interaction.WorkspaceID = oobTest.WorkspaceID
		d.db.Save(&interaction)
		issue := GetIssueTemplateByCode(oobTest.Code)
		issue.Payload = oobTest.Payload
		issue.URL = oobTest.Target
		issue.WorkspaceID = oobTest.WorkspaceID
		if oobTest.HistoryItem != nil {
			issue.Requests = append(issue.Requests, *oobTest.HistoryItem)
		}
		issue.Interactions = append(issue.Interactions, interaction)

		var sb strings.Builder
		sb.WriteString("An out of band " + interaction.Protocol + " interaction has been detected by inserting the following payload `" + oobTest.Payload + "` in " + oobTest.InsertionPoint + "\n\n")
		sb.WriteString("The interaction originated from " + interaction.RemoteAddress + " and was performed at " + interaction.Timestamp.String() + ".\n\nFind below the request data:\n")
		sb.WriteString(interaction.RawRequest + "\n\n")
		sb.WriteString("The server responded with the following data:\n")
		sb.WriteString(interaction.RawResponse + "\n")
		details := sb.String()
		if oobTest.HistoryID != nil && *oobTest.HistoryID > 0 {
			history, _ := d.GetHistory(*oobTest.HistoryID)
			issue.StatusCode = history.StatusCode
			issue.HTTPMethod = history.Method
			issue.Request = history.RawRequest
			issue.Response = history.RawResponse
			issue.Confidence = 80
			issue.Details = details
		}
		d.CreateIssue(*issue)
	}
	return oobTest, result.Error
}

type InteractionsFilter struct {
	QTypes      []string
	Protocols   []string
	FullIDs     []string
	Pagination  Pagination
	WorkspaceID uint
}

// ListInteractions Lists interactions
func (d *DatabaseConnection) ListInteractions(filter InteractionsFilter) (items []*OOBInteraction, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.QTypes) > 0 {
		filterQuery["qtype"] = filter.QTypes
	}
	if len(filter.Protocols) > 0 {
		filterQuery["protocol"] = filter.Protocols
	}

	if len(filter.FullIDs) > 0 {
		filterQuery["full_id"] = filter.FullIDs
	}

	if filter.WorkspaceID > 0 {
		filterQuery["workspace_id"] = filter.WorkspaceID
	}
	if filterQuery != nil && len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&OOBInteraction{}).Where(filterQuery).Count(&count)

	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&OOBInteraction{}).Count(&count)
	}

	log.Debug().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Msg("Getting interaction items")

	return items, count, err
}
