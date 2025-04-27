package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

func parseWorkspaceID(c *fiber.Ctx) (uint, error) {
	unparsedWorkspaceID := c.Query("workspace")
	workspaceID64, err := strconv.ParseUint(unparsedWorkspaceID, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing workspace parameter query")
		return 0, err

	}

	workspaceID := uint(workspaceID64)
	workspaceExists, _ := db.Connection().WorkspaceExists(workspaceID)
	if !workspaceExists {
		return 0, errors.New("Invalid workspace")
	}
	return workspaceID, nil
}

func parsePlaygroundCollectionID(c *fiber.Ctx) (uint, error) {
	unparsed := c.Query("collection")
	if unparsed == "" {
		return 0, nil
	}
	collectionID64, err := strconv.ParseUint(unparsed, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing playground collection parameter query")
		return 0, err
	}

	collectionID := uint(collectionID64)
	if collectionID == 0 {
		return 0, nil
	}
	_, err = db.Connection().GetPlaygroundCollection(collectionID)
	if err != nil {
		return 0, err
	}
	return collectionID, nil
}

func parsePlaygroundSessionID(c *fiber.Ctx) (uint, error) {
	unparsed := c.Query("playground_session")
	if unparsed == "" {
		return 0, nil
	}
	sessionID64, err := strconv.ParseUint(unparsed, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing playground session parameter query")
		return 0, err
	}

	sessionID := uint(sessionID64)
	if sessionID == 0 {
		return 0, nil
	}
	_, err = db.Connection().GetPlaygroundSession(sessionID)
	if err != nil {
		return 0, err
	}
	return sessionID, nil
}

func parseTaskID(c *fiber.Ctx) (uint, error) {
	unparsed := c.Query("task")
	if unparsed == "" {
		return 0, nil
	}
	taskID64, err := strconv.ParseUint(unparsed, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing task parameter query")
		return 0, err

	}

	taskID := uint(taskID64)
	if taskID == 0 {
		return 0, nil
	}
	taskExists, _ := db.Connection().TaskExists(taskID)
	if !taskExists {
		return 0, errors.New("Invalid task")
	}
	return taskID, nil
}

func parseTaskJobID(c *fiber.Ctx) (uint, error) {
	unparsed := c.Query("taskjob")
	if unparsed == "" {
		return 0, nil
	}
	taskJobID64, err := strconv.ParseUint(unparsed, 10, strconv.IntSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing task job parameter query")
		return 0, err
	}

	taskJobID := uint(taskJobID64)
	if taskJobID == 0 {
		return 0, nil
	}
	taskJobExists, _ := db.Connection().TaskJobExists(taskJobID)
	if !taskJobExists {
		return 0, errors.New("Invalid task job")
	}
	return taskJobID, nil
}

func stringToUintSlice(input string, acceptedValues []uint, silentFail bool) ([]uint, error) {
	output := make([]uint, 0)

	if input == "" {
		return output, nil
	}
	for _, item := range strings.Split(input, ",") {
		if item == "" {
			if silentFail {
				continue
			}
			return nil, errors.New("Invalid value")
		}
		parsed, err := parseUint(item)
		if err != nil {
			if silentFail {
				continue
			}
			return nil, err
		}
		if len(acceptedValues) > 0 && !lib.SliceContainsUint(acceptedValues, parsed) {
			if silentFail {
				continue
			}
			log.Info().Uint("value", parsed).Str("input", input).Msg("Invalid value")
			return nil, errors.New("Invalid value")
		}
		output = append(output, parsed)
	}
	return output, nil
}

func stringToIntSlice(input string, acceptedValues []int, silentFail bool) ([]int, error) {
	output := make([]int, 0)

	if input == "" {
		return output, nil
	}

	for _, item := range strings.Split(input, ",") {
		if item == "" {
			if silentFail {
				continue
			}
			return nil, errors.New("Invalid value")
		}

		parsed, err := parseInt(item)
		if err != nil {
			if silentFail {
				continue
			}
			return nil, err
		}
		if len(acceptedValues) > 0 && !lib.SliceContainsInt(acceptedValues, parsed) {
			if silentFail {
				continue
			}
			return nil, errors.New("Invalid value")
		}
		output = append(output, parsed)
	}

	return output, nil
}

func stringToSlice(input string, acceptedValues []string, silentFail bool) ([]string, error) {
	output := make([]string, 0)

	if input == "" {
		return output, nil
	}

	for _, item := range strings.Split(input, ",") {
		if item == "" {
			if silentFail {
				continue
			}
			return nil, errors.New("Invalid value")
		}

		if len(acceptedValues) > 0 && !lib.SliceContains(acceptedValues, item) {
			if silentFail {
				continue
			}
			return nil, errors.New("Invalid value")
		}
		output = append(output, item)
	}

	return output, nil
}

func parseInt(input string) (int, error) {
	return strconv.Atoi(input)
}

func parseUint(input string) (uint, error) {
	val, err := strconv.ParseUint(input, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(val), nil
}
