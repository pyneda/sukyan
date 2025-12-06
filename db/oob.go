package db

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
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
	Task              Task      `json:"-" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	TaskID            *uint     `json:"task_id" gorm:"index"`
	TaskJobID         *uint     `json:"task_job_id" gorm:"index"`
	TaskJob           *TaskJob  `json:"-" gorm:"foreignKey:TaskJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ScanID            *uint     `json:"scan_id" gorm:"index"`
	Scan              *Scan     `json:"-" gorm:"foreignKey:ScanID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	ScanJobID         *uint     `json:"scan_job_id" gorm:"index"`
	ScanJob           *ScanJob  `json:"-" gorm:"foreignKey:ScanJobID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	IssueID           *uint     `json:"issue_id" gorm:"index"`
	Issue             *Issue    `json:"-" gorm:"foreignKey:IssueID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
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

	// Handle foreign key constraints: set pointers to nil if they point to 0
	// This is needed because the new scan engine (V2) uses ScanID/ScanJobID instead of TaskID/TaskJobID
	// When TaskID or TaskJobID is 0, passing a pointer to 0 violates the foreign key constraint
	if item.TaskID != nil && *item.TaskID == 0 {
		item.TaskID = nil
	}
	if item.TaskJobID != nil && *item.TaskJobID == 0 {
		item.TaskJobID = nil
	}
	if item.ScanID != nil && *item.ScanID == 0 {
		item.ScanID = nil
	}
	if item.ScanJobID != nil && *item.ScanJobID == 0 {
		item.ScanJobID = nil
	}
	if item.HistoryID != nil && *item.HistoryID == 0 {
		item.HistoryID = nil
	}

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

// BuildInteractionsDetails generates a details string from multiple OOB interactions
func BuildInteractionsDetails(interactions []OOBInteraction, payload string, insertionPoint string) string {
	var sb strings.Builder

	if len(interactions) == 0 {
		return ""
	}

	if len(interactions) == 1 {
		i := interactions[0]
		sb.WriteString("An out of band " + i.Protocol + " interaction has been detected by inserting the following payload `" + payload + "` in " + insertionPoint + "\n\n")
		sb.WriteString("The interaction originated from " + i.RemoteAddress + " and was performed at " + i.Timestamp.String() + ".\n\nFind below the interaction request data:\n")
		sb.WriteString(i.RawRequest + "\n\n")
		sb.WriteString("The server responded with the following data:\n")
		sb.WriteString(i.RawResponse + "\n")
	} else {
		sb.WriteString(fmt.Sprintf("Multiple out of band interactions (%d total) have been detected by inserting the following payload `%s` in %s\n\n", len(interactions), payload, insertionPoint))

		// Group interactions by protocol for summary
		protocolCounts := make(map[string]int)
		for _, i := range interactions {
			protocolCounts[i.Protocol]++
		}

		sb.WriteString("**Summary of protocols:**\n")
		for protocol, count := range protocolCounts {
			sb.WriteString(fmt.Sprintf("- %s: %d interaction(s)\n", protocol, count))
		}
		sb.WriteString("\n---\n\n")

		for idx, i := range interactions {
			sb.WriteString(fmt.Sprintf("### Interaction %d (%s)\n\n", idx+1, i.Protocol))
			sb.WriteString("The interaction originated from " + i.RemoteAddress + " and was performed at " + i.Timestamp.String() + ".\n\n")
			sb.WriteString("**Request data:**\n```\n" + i.RawRequest + "\n```\n\n")
			sb.WriteString("**Response data:**\n```\n" + i.RawResponse + "\n```\n\n")
			if idx < len(interactions)-1 {
				sb.WriteString("---\n\n")
			}
		}
	}

	return sb.String()
}

func (d *DatabaseConnection) MatchInteractionWithOOBTest(interaction OOBInteraction) (OOBTest, error) {
	oobTest := OOBTest{}
	fullID := strings.ToLower(interaction.FullID)
	// Remove protocol prefixes like "imap://", "http://", "ldap://", etc.
	if idx := strings.Index(fullID, "://"); idx != -1 {
		fullID = fullID[idx+3:]
	}

	log.Debug().Str("extracted_id", fullID).Str("original_full_id", interaction.FullID).Msg("Attempting to match OOB test")

	// Use a transaction with row-level locking to prevent race conditions
	// when multiple interactions arrive simultaneously for the same OOB test
	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Lock the OOBTest row for update to prevent concurrent modifications
		result := tx.Set("gorm:query_option", "FOR UPDATE").Where("interaction_full_id = ?", fullID).First(&oobTest)
		if result.Error != nil {
			log.Error().Err(result.Error).Str("extracted_id", fullID).Interface("interaction", interaction).Msg("Failed to find OOBTest")
			return result.Error
		}

		log.Info().Interface("oobTest", oobTest).Interface("interaction", interaction).Msg("Matched Interaction and OOBTest")

		// Update interaction with OOBTest reference
		interaction.OOBTestID = &oobTest.ID
		interaction.WorkspaceID = oobTest.WorkspaceID

		// Check if an issue already exists for this OOBTest
		if oobTest.IssueID != nil && *oobTest.IssueID > 0 {
			// Issue already exists - add interaction to it and regenerate details
			log.Info().Uint("issue_id", *oobTest.IssueID).Uint("oob_test_id", oobTest.ID).Msg("Adding interaction to existing issue")

			// Set the IssueID on the interaction and save it
			interaction.IssueID = oobTest.IssueID
			if err := tx.Save(&interaction).Error; err != nil {
				log.Error().Err(err).Msg("Failed to save interaction")
				return err
			}

			// Fetch all interactions for this issue to regenerate details
			var allInteractions []OOBInteraction
			if err := tx.Where("issue_id = ?", *oobTest.IssueID).Order("timestamp ASC").Find(&allInteractions).Error; err != nil {
				log.Error().Err(err).Msg("Failed to fetch interactions for issue")
				return err
			}

			// Regenerate details with all interactions
			newDetails := BuildInteractionsDetails(allInteractions, oobTest.Payload, oobTest.InsertionPoint)

			// Update the issue's details
			if err := tx.Model(&Issue{}).Where("id = ?", *oobTest.IssueID).Update("details", newDetails).Error; err != nil {
				log.Error().Err(err).Uint("issue_id", *oobTest.IssueID).Msg("Failed to update issue details")
				return err
			}

			log.Info().Uint("issue_id", *oobTest.IssueID).Int("total_interactions", len(allInteractions)).Msg("Updated issue with new interaction")
			return nil
		}

		// No existing issue - create a new one
		if err := tx.Save(&interaction).Error; err != nil {
			log.Error().Err(err).Msg("Failed to save interaction")
			return err
		}

		issue := GetIssueTemplateByCode(oobTest.Code)
		issue.Payload = oobTest.Payload
		issue.URL = oobTest.Target
		issue.WorkspaceID = oobTest.WorkspaceID
		issue.TaskID = oobTest.TaskID
		issue.TaskJobID = oobTest.TaskJobID
		issue.ScanID = oobTest.ScanID
		issue.ScanJobID = oobTest.ScanJobID

		// Load history item if available
		if oobTest.HistoryID != nil && *oobTest.HistoryID > 0 {
			var history History
			if err := tx.First(&history, *oobTest.HistoryID).Error; err == nil {
				issue.Requests = append(issue.Requests, history)
				issue.StatusCode = history.StatusCode
				issue.HTTPMethod = history.Method
				issue.Request = history.RawRequest
				issue.Response = history.RawResponse
			}
		}

		issue.Confidence = 80
		issue.Details = BuildInteractionsDetails([]OOBInteraction{interaction}, oobTest.Payload, oobTest.InsertionPoint)

		// Create the issue
		if err := tx.Create(issue).Error; err != nil {
			log.Error().Err(err).Str("issue_code", string(oobTest.Code)).Str("issue_title", issue.Title).Msg("Failed to create issue from OOB test")
			return err
		}

		log.Info().Uint("issue_id", issue.ID).Str("issue_code", string(oobTest.Code)).Str("issue_title", issue.Title).Msg("Created issue from OOB test")

		// Update interaction with the new issue ID
		if err := tx.Model(&interaction).Update("issue_id", issue.ID).Error; err != nil {
			log.Error().Err(err).Msg("Failed to update interaction with issue ID")
			return err
		}

		// Update OOBTest with the issue ID for future interactions
		if err := tx.Model(&oobTest).Update("issue_id", issue.ID).Error; err != nil {
			log.Error().Err(err).Msg("Failed to update OOBTest with issue ID")
			return err
		}

		return nil
	})

	if err != nil {
		return oobTest, err
	}

	return oobTest, nil
}

type InteractionsFilter struct {
	QTypes          []string   `json:"qtypes" validate:"omitempty,dive,max=50"`
	Protocols       []string   `json:"protocols" validate:"omitempty,dive,max=50"`
	FullIDs         []string   `json:"full_ids" validate:"omitempty,dive,max=500"`
	RemoteAddresses []string   `json:"remote_addresses" validate:"omitempty,dive,max=50"`
	OOBTestIDs      []uint     `json:"oob_test_ids" validate:"omitempty,dive,min=1"`
	IssueIDs        []uint     `json:"issue_ids" validate:"omitempty,dive,min=1"`
	ScanIDs         []uint     `json:"scan_ids" validate:"omitempty,dive,min=1"`
	ScanJobIDs      []uint     `json:"scan_job_ids" validate:"omitempty,dive,min=1"`
	Pagination      Pagination `json:"pagination"`
	WorkspaceID     uint       `json:"workspace_id" validate:"omitempty,min=1"`
}

// ListInteractions Lists interactions
func (d *DatabaseConnection) ListInteractions(filter InteractionsFilter) (items []*OOBInteraction, count int64, err error) {
	query := d.db.Model(&OOBInteraction{})

	if len(filter.QTypes) > 0 {
		query = query.Where("q_type IN ?", filter.QTypes)
	}
	if len(filter.Protocols) > 0 {
		query = query.Where("protocol IN ?", filter.Protocols)
	}
	if len(filter.FullIDs) > 0 {
		query = query.Where("full_id IN ?", filter.FullIDs)
	}
	if len(filter.RemoteAddresses) > 0 {
		query = query.Where("remote_address IN ?", filter.RemoteAddresses)
	}
	if len(filter.OOBTestIDs) > 0 {
		query = query.Where("oob_test_id IN ?", filter.OOBTestIDs)
	}
	if len(filter.IssueIDs) > 0 {
		query = query.Where("issue_id IN ?", filter.IssueIDs)
	}
	if filter.WorkspaceID > 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}

	// ScanIDs and ScanJobIDs are on the related OOBTest, so use subquery
	if len(filter.ScanIDs) > 0 {
		query = query.Where("oob_test_id IN (SELECT id FROM oob_tests WHERE scan_id IN ?)", filter.ScanIDs)
	}
	if len(filter.ScanJobIDs) > 0 {
		query = query.Where("oob_test_id IN (SELECT id FROM oob_tests WHERE scan_job_id IN ?)", filter.ScanJobIDs)
	}

	if err := query.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count interactions")
		return nil, 0, err
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error

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
	ScanIDs            []uint     `json:"scan_ids" validate:"omitempty,dive,min=1"`
	ScanJobIDs         []uint     `json:"scan_job_ids" validate:"omitempty,dive,min=1"`
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
	ScanID             uint       `json:"scan_id" validate:"omitempty,min=1"`
	ScanJobID          uint       `json:"scan_job_id" validate:"omitempty,min=1"`
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
	if len(filter.ScanIDs) > 0 {
		query = query.Where("scan_id IN ?", filter.ScanIDs)
	}
	if len(filter.ScanJobIDs) > 0 {
		query = query.Where("scan_job_id IN ?", filter.ScanJobIDs)
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
	if filter.ScanID > 0 {
		query = query.Where("scan_id = ?", filter.ScanID)
	}
	if filter.ScanJobID > 0 {
		query = query.Where("scan_job_id = ?", filter.ScanJobID)
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
