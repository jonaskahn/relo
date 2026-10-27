// Model listing: one id reaches the console once, however it was spelled.
package integration

import "testing"

// TestIntegrationModelsKeepsOneEntryPerID covers a document that carries the
// same model in both of its arrays: the console keys its picker on the id, so a
// second copy of it is a duplicate key rather than another model.
func TestIntegrationModelsKeepsOneEntryPerID(t *testing.T) {
	body := []byte(`{"models":[{"id":"gpt-6.1"}],"data":[{"id":"gpt-6.1"},{"id":"gpt-6.2"}]}`)
	models := integrationModelsOf(body)

	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	if len(ids) != 2 || ids[0] != "gpt-6.1" || ids[1] != "gpt-6.2" {
		t.Fatalf("models = %v, want each id listed once", ids)
	}
}
