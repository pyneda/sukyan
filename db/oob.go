package db

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
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
	HistoryItem       *History  `json:"-" gorm:"foreignKey:HistoryID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	InteractionDomain string    `gorm:"index" json:"interaction_domain"`
	InteractionFullID string    `gorm:"index" json:"interaction_id"`
	Payload           string    `json:"payload"`
	InsertionPoint    string    `json:"insertion_point"`
	Workspace         Workspace `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID       *uint     `json:"workspace_id"`
	Task              Task      `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	TaskID            *uint     `json:"task_id"`
	TaskJobID         *uint     `json:"task_job_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	TaskJob           TaskJob   `json:"-" gorm:"foreignKey:TaskJobID"`
	Note              string    `json:"note"`
}

func (o OOBTest) TableHeaders() []string {
	return []string{"ID", "Test Name", "Target", "Interaction Domain", "Interaction Full ID", "Payload", "Insertion Point", "Workspace ID", "Task ID"}
}

func (o OOBTest) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", o.ID),
		o.TestName,
		o.Target,
		o.InteractionDomain,
		o.InteractionFullID,
		o.Payload,
		o.InsertionPoint,
		formatUintPointer(o.WorkspaceID),
		formatUintPointer(o.TaskID),
	}
}

func (o OOBTest) String() string {
	return fmt.Sprintf(
		"ID: %d\nTest Name: %s\nTarget: %s\nInteraction Domain: %s\nInteraction Full ID: %s\nPayload: %s\nInsertion Point: %s\nWorkspace ID: %s\nTask ID: %s",
		o.ID, o.TestName, o.Target, o.InteractionDomain, o.InteractionFullID, o.Payload, o.InsertionPoint, formatUintPointer(o.WorkspaceID), formatUintPointer(o.TaskID),
	)
}

func (o OOBTest) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sTest Name:%s %s\n%sTarget:%s %s\n%sInteraction Domain:%s %s\n%sInteraction Full ID:%s %s\n%sPayload:%s %s\n%sInsertion Point:%s %s\n%sWorkspace ID:%s %s\n%sTask ID:%s %s\n",
		lib.Blue, lib.ResetColor, o.ID,
		lib.Blue, lib.ResetColor, o.TestName,
		lib.Blue, lib.ResetColor, o.Target,
		lib.Blue, lib.ResetColor, o.InteractionDomain,
		lib.Blue, lib.ResetColor, o.InteractionFullID,
		lib.Blue, lib.ResetColor, o.Payload,
		lib.Blue, lib.ResetColor, o.InsertionPoint,
		lib.Blue, lib.ResetColor, formatUintPointer(o.WorkspaceID),
		lib.Blue, lib.ResetColor, formatUintPointer(o.TaskID),
	)
}

// CreateOOBTest saves an OOBTest to the database
func (d *DatabaseConnection) CreateOOBTest(item OOBTest) (OOBTest, error) {
	item.InteractionFullID = strings.ToLower(item.InteractionFullID)

	// Check if payload contains invalid UTF-8 sequences (binary data)
	if !utf8.ValidString(item.Payload) {
		log.Debug().Str("original_payload_length", fmt.Sprintf("%d bytes", len(item.Payload))).Msg("OOBTest payload contains binary data, encoding as base64")
		encodedPayload := base64.StdEncoding.EncodeToString([]byte(item.Payload))
		item.Payload = encodedPayload

		transformationNote := fmt.Sprintf("Original payload contained binary data and was base64 encoded (original length: %d bytes)", len([]byte(item.Payload)))
		if item.Note == "" {
			item.Note = transformationNote
		} else {
			item.Note = item.Note + "\n" + transformationNote
		}
	}

	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("item", item).Msg("Failed to create OOBTest")
	}
	return item, result.Error
}

type OOBInteraction struct {
	BaseModel
	OOBTestID *uint   `json:"oob_test_id"`
	OOBTest   OOBTest `gorm:"foreignKey:OOBTestID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"oob_test"`

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

func (o OOBInteraction) TableHeaders() []string {
	return []string{"ID", "Protocol", "Full ID", "Unique ID", "QType", "Timestamp", "Remote Address", "Workspace ID", "Issue ID"}
}

func (o OOBInteraction) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", o.ID),
		o.Protocol,
		o.FullID,
		o.UniqueID,
		o.QType,
		o.Timestamp.Format(time.RFC3339),
		o.RemoteAddress,
		formatUintPointer(o.WorkspaceID),
		formatUintPointer(o.IssueID),
	}
}

func (o OOBInteraction) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sProtocol:%s %s\n%sFull ID:%s %s\n%sUnique ID:%s %s\n%sQType:%s %s\n%sRaw Request:%s %s\n%sRaw Response:%s %s\n%sRemote Address:%s %s\n%sTimestamp:%s %s\n%sWorkspace ID:%s %s\n%sIssue ID:%s %s\n",
		lib.Blue, lib.ResetColor, o.ID,
		lib.Blue, lib.ResetColor, o.Protocol,
		lib.Blue, lib.ResetColor, o.FullID,
		lib.Blue, lib.ResetColor, o.UniqueID,
		lib.Blue, lib.ResetColor, o.QType,
		lib.Blue, lib.ResetColor, o.RawRequest,
		lib.Blue, lib.ResetColor, o.RawResponse,
		lib.Blue, lib.ResetColor, o.RemoteAddress,
		lib.Blue, lib.ResetColor, o.Timestamp.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, formatUintPointer(o.WorkspaceID),
		lib.Blue, lib.ResetColor, formatUintPointer(o.IssueID),
	)
}

func (o OOBInteraction) String() string {
	return fmt.Sprintf(
		"ID: %d\nProtocol: %s\nFull ID: %s\nUnique ID: %s\nQType: %s\nRaw Request: %s\nRaw Response: %s\nRemote Address: %s\nTimestamp: %s\nWorkspace ID: %s\nIssue ID: %s",
		o.ID, o.Protocol, o.FullID, o.UniqueID, o.QType, o.RawRequest, o.RawResponse, o.RemoteAddress, o.Timestamp.Format(time.RFC3339), formatUintPointer(o.WorkspaceID), formatUintPointer(o.IssueID),
	)
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
	// Remove protocol prefixes like "imap://", "http://", "ldap://", etc.
	if idx := strings.Index(fullID, "://"); idx != -1 {
		fullID = fullID[idx+3:]
	}

	log.Debug().Str("extracted_id", fullID).Str("original_full_id", interaction.FullID).Msg("Attempting to match OOB test")

	result := d.db.Where("interaction_full_id = ?", fullID).First(&oobTest)
	if result.Error != nil {
		log.Error().Err(result.Error).Str("extracted_id", fullID).Interface("interaction", interaction).Msg("Failed to find OOBTest")
	} else {
		log.Info().Interface("oobTest", oobTest).Interface("interaction", interaction).Msg("Matched Interaction and OOBTest")
		interaction.OOBTestID = &oobTest.ID
		interaction.WorkspaceID = oobTest.WorkspaceID
		d.db.Save(&interaction)
		issue := GetIssueTemplateByCode(oobTest.Code)
		issue.Payload = oobTest.Payload
		issue.URL = oobTest.Target
		issue.WorkspaceID = oobTest.WorkspaceID
		issue.TaskID = oobTest.TaskID
		issue.TaskJobID = oobTest.TaskJobID
		if oobTest.HistoryItem != nil {
			issue.Requests = append(issue.Requests, *oobTest.HistoryItem)
		}
		issue.Interactions = append(issue.Interactions, interaction)

		var sb strings.Builder
		sb.WriteString("An out of band " + interaction.Protocol + " interaction has been detected by inserting the following payload `" + oobTest.Payload + "` in " + oobTest.InsertionPoint + "\n\n")
		sb.WriteString("The interaction originated from " + interaction.RemoteAddress + " and was performed at " + interaction.Timestamp.String() + ".\n\nFind below the interaction request data:\n")
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
		createdIssue, err := d.CreateIssue(*issue)
		if err != nil {
			log.Error().Err(err).Str("issue_code", string(oobTest.Code)).Str("issue_title", issue.Title).Msg("Failed to create issue from OOB test")
		} else {
			log.Info().Uint("issue_id", createdIssue.ID).Str("issue_code", string(oobTest.Code)).Str("issue_title", issue.Title).Msg("Created issue from OOB test")
		}
	}
	return oobTest, result.Error
}

type InteractionsFilter struct {
	QTypes          []string   `json:"qtypes" validate:"omitempty,dive,max=50"`
	Protocols       []string   `json:"protocols" validate:"omitempty,dive,max=50"`
	FullIDs         []string   `json:"full_ids" validate:"omitempty,dive,max=500"`
	RemoteAddresses []string   `json:"remote_addresses" validate:"omitempty,dive,max=50"`
	OOBTestIDs      []uint     `json:"oob_test_ids" validate:"omitempty,dive,min=1"`
	Pagination      Pagination `json:"pagination"`
	WorkspaceID     uint       `json:"workspace_id" validate:"omitempty,min=1"`
}

// ListInteractions Lists interactions
func (d *DatabaseConnection) ListInteractions(filter InteractionsFilter) (items []*OOBInteraction, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.QTypes) > 0 {
		filterQuery["q_type"] = filter.QTypes
	}
	if len(filter.Protocols) > 0 {
		filterQuery["protocol"] = filter.Protocols
	}

	if len(filter.FullIDs) > 0 {
		filterQuery["full_id"] = filter.FullIDs
	}

	if len(filter.RemoteAddresses) > 0 {
		filterQuery["remote_address"] = filter.RemoteAddresses
	}

	if len(filter.OOBTestIDs) > 0 {
		filterQuery["oob_test_id"] = filter.OOBTestIDs
	}

	if filter.WorkspaceID > 0 {
		filterQuery["workspace_id"] = filter.WorkspaceID
	}
	if len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&OOBInteraction{}).Where(filterQuery).Count(&count)

	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&OOBInteraction{}).Count(&count)
	}

	log.Debug().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Msg("Getting interaction items")

	if err != nil {
		log.Error().Err(err).Msg("Failed to list interactions")
	}
	return items, count, err
}

type OOBTestsFilter struct {
	Query              string     `json:"query" validate:"omitempty,max=500"`
	TestNames          []string   `json:"test_names" validate:"omitempty,dive,max=200"`
	Targets            []string   `json:"targets" validate:"omitempty,dive,max=2000"`
	InteractionDomains []string   `json:"interaction_domains" validate:"omitempty,dive,max=253"`
	InteractionFullIDs []string   `json:"interaction_full_ids" validate:"omitempty,dive,max=500"`
	Payloads           []string   `json:"payloads" validate:"omitempty,dive,max=5000"`
	InsertionPoints    []string   `json:"insertion_points" validate:"omitempty,dive,max=100"`
	Codes              []string   `json:"codes" validate:"omitempty,dive,max=50"`
	HistoryIDs         []uint     `json:"history_ids" validate:"omitempty,dive,min=1"`
	TaskIDs            []uint     `json:"task_ids" validate:"omitempty,dive,min=1"`
	TaskJobIDs         []uint     `json:"task_job_ids" validate:"omitempty,dive,min=1"`
	HasInteractions    *bool      `json:"has_interactions" validate:"omitempty"`
	CreatedAfter       *time.Time `json:"created_after" validate:"omitempty"`
	CreatedBefore      *time.Time `json:"created_before" validate:"omitempty"`
	UpdatedAfter       *time.Time `json:"updated_after" validate:"omitempty"`
	UpdatedBefore      *time.Time `json:"updated_before" validate:"omitempty"`
	SortBy             string     `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at test_name target"`
	SortOrder          string     `json:"sort_order" validate:"omitempty,oneof=asc desc"`
	Pagination         Pagination `json:"pagination"`
	WorkspaceID        uint       `json:"workspace_id" validate:"omitempty,min=1"`
	TaskID             uint       `json:"task_id" validate:"omitempty,min=1"`
	TaskJobID          uint       `json:"task_job_id" validate:"omitempty,min=1"`
}

// ListOOBTests Lists OOB tests
func (d *DatabaseConnection) ListOOBTests(filter OOBTestsFilter) (items []*OOBTest, count int64, err error) {
	query := d.db.Model(&OOBTest{})

	if filter.Query != "" {
		searchQuery := "%" + filter.Query + "%"
		query = query.Where("test_name ILIKE ? OR target ILIKE ? OR payload ILIKE ? OR insertion_point ILIKE ? OR interaction_domain ILIKE ? OR note ILIKE ? OR code ILIKE ?",
			searchQuery, searchQuery, searchQuery, searchQuery, searchQuery, searchQuery, searchQuery)
	}

	if len(filter.TestNames) > 0 {
		query = query.Where("test_name IN ?", filter.TestNames)
	}
	if len(filter.Targets) > 0 {
		query = query.Where("target IN ?", filter.Targets)
	}
	if len(filter.InteractionDomains) > 0 {
		query = query.Where("interaction_domain IN ?", filter.InteractionDomains)
	}
	if len(filter.InteractionFullIDs) > 0 {
		query = query.Where("interaction_full_id IN ?", filter.InteractionFullIDs)
	}
	if len(filter.Payloads) > 0 {
		query = query.Where("payload IN ?", filter.Payloads)
	}
	if len(filter.InsertionPoints) > 0 {
		query = query.Where("insertion_point IN ?", filter.InsertionPoints)
	}
	if len(filter.Codes) > 0 {
		query = query.Where("code IN ?", filter.Codes)
	}
	if len(filter.HistoryIDs) > 0 {
		query = query.Where("history_id IN ?", filter.HistoryIDs)
	}
	if len(filter.TaskIDs) > 0 {
		query = query.Where("task_id IN ?", filter.TaskIDs)
	}
	if len(filter.TaskJobIDs) > 0 {
		query = query.Where("task_job_id IN ?", filter.TaskJobIDs)
	}

	if filter.HasInteractions != nil {
		if *filter.HasInteractions {
			query = query.Where("EXISTS (SELECT 1 FROM oob_interactions WHERE oob_interactions.oob_test_id = oob_tests.id)")
		} else {
			query = query.Where("NOT EXISTS (SELECT 1 FROM oob_interactions WHERE oob_interactions.oob_test_id = oob_tests.id)")
		}
	}

	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", *filter.CreatedAfter)
	}
	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", *filter.CreatedBefore)
	}
	if filter.UpdatedAfter != nil {
		query = query.Where("updated_at >= ?", *filter.UpdatedAfter)
	}
	if filter.UpdatedBefore != nil {
		query = query.Where("updated_at <= ?", *filter.UpdatedBefore)
	}

	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID > 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	if filter.TaskJobID > 0 {
		query = query.Where("task_job_id = ?", filter.TaskJobID)
	}

	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count OOB tests")
		return nil, 0, err
	}

	if filter.Pagination.PageSize > 0 && filter.Pagination.Page > 0 {
		query = query.Scopes(Paginate(&filter.Pagination))
	}

	sortBy := "created_at"
	if filter.SortBy != "" {
		sortBy = filter.SortBy
	}
	sortOrder := "desc"
	if filter.SortOrder != "" {
		sortOrder = filter.SortOrder
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	if err := query.Find(&items).Error; err != nil {
		log.Error().Err(err).Msg("Failed to list OOB tests")
		return nil, 0, err
	}

	log.Debug().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Msg("Getting OOB test items")

	return items, count, err
}

// UpdateOOBTestHistoryID updates an existing OOBTest with history ID
func (d *DatabaseConnection) UpdateOOBTestHistoryID(oobTestID uint, historyID *uint) error {
	result := d.db.Model(&OOBTest{}).Where("id = ?", oobTestID).Update("history_id", historyID)
	if result.Error != nil {
		log.Error().Err(result.Error).Uint("history_id", *historyID).Uint("oob_test_id", oobTestID).Msg("Failed to update OOBTest history ID")
	}
	return result.Error
}
