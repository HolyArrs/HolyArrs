package musictags

import (
	_ "embed"
	"encoding/json"
)

//go:embed recipes.json
var recipesJSON []byte

// Recipes answers every key's recipe, in Browse order (genres, moods,
// seasons, decades, activities, then substyles).
func Recipes() ([]Recipe, error) {
	var rs []Recipe
	if err := json.Unmarshal(recipesJSON, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}
