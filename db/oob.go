package db

import (
	"fmt"
	"strings"
	"time"

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
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("item", item).Msg("Failed to create OOBTest")
	}
	return item, result.Error
}

type OOBInteraction struct {
	BaseModel
	OOBTestID *uint   `json:"oob_test_id"`
	OOBTest   OOBTest `gorm:"foreignKey:OOBTestID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`

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
