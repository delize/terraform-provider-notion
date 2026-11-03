package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jomei/notionapi"
)

func TestBasicPropertyConfig_PhoneNumber(t *testing.T) {
	r := newDatabasePropertyBasicResource("phone_number", notionapi.PropertyConfigTypePhoneNumber)().(*DatabasePropertyBasicResource)
	b, err := json.Marshal(r.buildPropertyConfig())
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"type":"phone_number","phone_number":{}}` {
		t.Errorf("phone_number config = %s", b)
	}
}

// Every registered basic property resource must build a config. A type
// missing from buildPropertyConfig would send a null property, which
// deletes it.
func TestBasicPropertyConfig_EveryRegisteredType(t *testing.T) {
	p := &NotionProvider{}
	seen := 0
	for _, newRes := range p.Resources(context.Background()) {
		r, ok := newRes().(*DatabasePropertyBasicResource)
		if !ok {
			continue
		}
		seen++
		if r.buildPropertyConfig() == nil {
			t.Errorf("no property config for %s", r.typeName)
		}
		if r.typeName != string(r.propertyType) {
			t.Errorf("resource suffix %s doesn't match property type %s", r.typeName, r.propertyType)
		}
	}
	if seen != 11 {
		t.Errorf("found %d basic property resources, want 11", seen)
	}
}
