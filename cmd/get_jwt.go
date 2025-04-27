package cmd

import (
	"fmt"

	"github.com/go-playground/validator/v10"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	filterAlgorithm string
	filterIssuer    string
	filterSubject   string
	filterAudience  string
	filterSortBy    string
	filterSortOrder string
)

var getJwtCmd = &cobra.Command{
	Use:     "jwt",
	Aliases: []string{"jwts", "j"},
	Short:   "List JSON Web Tokens",
	Run: func(cmd *cobra.Command, args []string) {
		filters := db.JwtFilters{
			Algorithm:   filterAlgorithm,
			Issuer:      filterIssuer,
			Subject:     filterSubject,
			Audience:    filterAudience,
			SortBy:      filterSortBy,
			SortOrder:   filterSortOrder,
			WorkspaceID: workspaceID,
		}

		validate := validator.New()
		if err := validate.Struct(filters); err != nil {
			errors := make(map[string]string)
			for _, err := range err.(validator.ValidationErrors) {
				errors[err.Field()] = fmt.Sprintf("Invalid value for %s", err.Field())
			}
			log.Error().Err(err).Msg("Validation failed")
			fmt.Printf("Error: Validation failed: %+v\n", errors)
			return
		}

		jwts, err := db.Connection().ListJsonWebTokens(filters)
		if err != nil {
			log.Error().Err(err).Msg("Error listing JWTs")
			return
		}

		if len(jwts) == 0 {
			fmt.Println("No JSON Web Tokens found")
			return
		}

		formatType, err := lib.ParseFormatType(format)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing format type")
			return
		}

		formattedOutput, err := lib.FormatOutput(jwts, formatType)
		if err != nil {
			log.Error().Err(err).Msg("Error formatting output")
			return
		}

		fmt.Println(formattedOutput)
	},
}

func init() {
	getCmd.AddCommand(getJwtCmd)
	getJwtCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Filter JWTs by workspace ID")
	getJwtCmd.Flags().StringVar(&filterAlgorithm, "algorithm", "", "Filter JWTs by algorithm")
	getJwtCmd.Flags().StringVar(&filterIssuer, "issuer", "", "Filter JWTs by issuer")
	getJwtCmd.Flags().StringVar(&filterSubject, "subject", "", "Filter JWTs by subject")
	getJwtCmd.Flags().StringVar(&filterAudience, "audience", "", "Filter JWTs by audience")
	getJwtCmd.Flags().StringVar(&filterSortBy, "sort-by", "", "Column to sort the JWTs")
	getJwtCmd.Flags().StringVar(&filterSortOrder, "sort-order", "asc", "Order of sorting (asc or desc)")
}
