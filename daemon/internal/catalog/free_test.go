package catalog

import "testing"

func TestIsFreeModel(t *testing.T) {
	free, paid := true, false
	zero, one := int64(0), int64(1)
	tests := []struct {
		name     string
		provider *bool
		listing  *bool
		id       string
		input    *int64
		output   *int64
		category Category
		want     bool
	}{
		{name: "free token hyphen", id: "model-free", category: CategoryChat, want: true},
		{name: "free token underscore", id: "model_free", category: CategoryChat, want: true},
		{name: "free token colon", id: "model:free", category: CategoryChat, want: true},
		{name: "free token slash", id: "vendor/free", category: CategoryChat, want: true},
		{name: "free token space", id: "free model", category: CategoryChat, want: true},
		{name: "freedom is not free", id: "freedom", category: CategoryChat, want: false},
		{name: "explicit zero rates", id: "model", input: &zero, output: &zero, category: CategoryChat, want: true},
		{name: "one-sided zero is unknown", id: "model", input: &zero, category: CategoryChat, want: false},
		{name: "missing prices are unknown", id: "model", category: CategoryChat, want: false},
		{name: "paid rates are not free", id: "model", input: &one, output: &one, category: CategoryChat, want: false},
		{name: "provider mark", id: "model", provider: &free, category: CategoryChat, want: true},
		{name: "listing mark", id: "model", listing: &free, category: CategoryChat, want: true},
		{name: "false mark does not count", id: "model", listing: &paid, category: CategoryChat, want: false},
		{name: "embedding excluded", id: "model-free", input: &zero, output: &zero, category: CategoryEmbedding, want: false},
		{name: "image excluded", id: "model-free", provider: &free, category: CategoryImage, want: false},
		{name: "audio excluded", id: "model-free", category: CategoryAudio, want: false},
		{name: "video excluded", id: "model-free", category: CategoryVideo, want: false},
		{name: "unknown excluded", id: "model-free", category: Category("unknown"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsFreeModel(test.provider, test.listing, test.id, test.input, test.output, test.category); got != test.want {
				t.Fatalf("IsFreeModel() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFreePricesProjectsMarksIntoZeroRates(t *testing.T) {
	free := true
	projected := FreePrices(Prices{}, &free)
	if projected.Input == nil || *projected.Input != 0 || projected.Output == nil || *projected.Output != 0 {
		t.Fatalf("projected prices = %+v, want explicit zero input and output", projected)
	}
	one := int64(1)
	kept := FreePrices(Prices{Input: &one}, &free)
	if kept.Input != &one || kept.Output != nil {
		t.Fatalf("explicit rates must not be overwritten: %+v", kept)
	}
}

func TestModelsDevProviderCountsConversationalFreeModels(t *testing.T) {
	free := true
	zero := int64(0)
	provider := ModelsDevProvider{Models: map[string]ModelsDevModel{
		"chat-free":   {ID: "chat-free", Category: "chat"},
		"reasoning":   {ID: "reasoning", Category: "reasoning", IsFree: &free},
		"embed":       {ID: "embed-free", Category: "embedding"},
		"paid":        {ID: "paid", Category: "chat"},
		"zero-priced": {ID: "zero-priced", Category: "chat", Prices: Prices{Input: &zero, Output: &zero}},
	}}
	if got := provider.FreeModels(); got != 3 {
		t.Fatalf("FreeModels() = %d, want 3", got)
	}
}
