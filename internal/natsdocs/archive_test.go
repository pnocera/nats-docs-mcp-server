package natsdocs

import "testing"

func TestArchiveCompatibilityAliases(t *testing.T) {
	indexedIDs := map[string]struct{}{
		"overview":                   {},
		"release_notes/whats_new":    {},
		"release_notes/whats_new_20": {},
		"nats-concepts/core-nats/publish-subscribe/pubsub":                         {},
		"nats-concepts/core-nats/publish-subscribe/pubsub_walkthrough":             {},
		"nats-concepts/jetstream/object-store/obj_store":                           {},
		"nats-concepts/jetstream/object-store/obj_walkthrough":                     {},
		"reference/nats-protocol/nats-protocol/nats-client-dev":                    {},
		"using-nats/developing-with-nats/connecting/security/creds":                {},
		"using-nats/developing-with-nats/reconnect/max":                            {},
		"using-nats/developing-with-nats/js/streams":                               {},
		"using-nats/jetstream/nats_api_reference":                                  {},
		"running-a-nats-service/configuration/jetstream-config/configuration_mgmt": {},
		"running-a-nats-service/configuration/securing_nats/jwt/resolver":          {},
		"running-a-nats-service/running/nats_docker/jetstream_docker":              {},
	}
	aliases := make(map[string]string)
	addArchiveCompatibilityAliases(aliases, indexedIDs)

	expected := map[string]string{
		"nats-concepts/overview":                                                      "overview",
		"release-notes/whats_new":                                                     "release_notes/whats_new",
		"release-notes/whats_new/whats_new_20":                                        "release_notes/whats_new_20",
		"nats-concepts/core-nats/pubsub":                                              "nats-concepts/core-nats/publish-subscribe/pubsub",
		"nats-concepts/core-nats/pubsub/pubsub_walkthrough":                           "nats-concepts/core-nats/publish-subscribe/pubsub_walkthrough",
		"nats-concepts/jetstream/obj_store":                                           "nats-concepts/jetstream/object-store/obj_store",
		"nats-concepts/jetstream/obj_store/obj_walkthrough":                           "nats-concepts/jetstream/object-store/obj_walkthrough",
		"reference/reference-protocols/nats-protocol/nats-client-dev":                 "reference/nats-protocol/nats-protocol/nats-client-dev",
		"using-nats/developer/connecting/creds":                                       "using-nats/developing-with-nats/connecting/security/creds",
		"using-nats/developer/connecting/reconnect/max":                               "using-nats/developing-with-nats/reconnect/max",
		"using-nats/developer/develop_jetstream/streams":                              "using-nats/developing-with-nats/js/streams",
		"reference/reference-protocols/nats_api_reference":                            "using-nats/jetstream/nats_api_reference",
		"running-a-nats-service/configuration/resource_management/configuration_mgmt": "running-a-nats-service/configuration/jetstream-config/configuration_mgmt",
		"running-a-nats-service/configuration/securing_nats/auth_intro/jwt/resolver":  "running-a-nats-service/configuration/securing_nats/jwt/resolver",
		"running-a-nats-service/nats_docker/jetstream_docker":                         "running-a-nats-service/running/nats_docker/jetstream_docker",
	}
	for alias, canonical := range expected {
		if got := aliases[alias]; got != canonical {
			t.Fatalf("expected alias %q to resolve to %q, got %q", alias, canonical, got)
		}
	}
}
