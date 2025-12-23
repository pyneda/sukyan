package generation

import "sort"

// TemplateMetadata holds aggregated information about all loaded templates
type TemplateMetadata struct {
	Platforms  []string `json:"platforms"`
	Categories []string `json:"categories"`
}

// GetTemplateMetadata returns all unique platforms and categories from loaded generators
func GetTemplateMetadata(generators []*PayloadGenerator) *TemplateMetadata {
	return &TemplateMetadata{
		Platforms:  GetAllPlatforms(generators),
		Categories: GetAllCategories(generators),
	}
}

// GetAllPlatforms returns all unique platforms from loaded generators, sorted alphabetically
func GetAllPlatforms(generators []*PayloadGenerator) []string {
	platformSet := make(map[string]struct{})
	for _, gen := range generators {
		for _, platform := range gen.Platforms {
			platformSet[platform] = struct{}{}
		}
	}

	platforms := make([]string, 0, len(platformSet))
	for platform := range platformSet {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	return platforms
}

// GetAllCategories returns all unique categories from loaded generators, sorted alphabetically
func GetAllCategories(generators []*PayloadGenerator) []string {
	categorySet := make(map[string]struct{})
	for _, gen := range generators {
		for _, category := range gen.Categories {
			categorySet[category] = struct{}{}
		}
	}

	categories := make([]string, 0, len(categorySet))
	for category := range categorySet {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}
