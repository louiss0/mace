package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	burnttoml "github.com/BurntSushi/toml"
	goccyyaml "github.com/goccy/go-yaml"
	yamlast "github.com/goccy/go-yaml/ast"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
)

func expectedOutput(source string) map[string]any {
	result, err := processor.New().Process(source)
	tAssert.NoError(err)
	return outputValue(result.Output)
}

func importedOutput(source string) map[string]any {
	result, err := processor.New().Process(source)
	tAssert.NoError(err)
	return outputValue(result.Output)
}

func canonicalJSON(value any) string {
	payload, err := json.Marshal(value)
	tAssert.NoError(err)
	return string(payload)
}

type importFixture struct {
	name string
	mace string
}

type quotedStringer string

func (value quotedStringer) String() string {
	return string(value)
}

func dataFixtures() []importFixture {
	return []importFixture{
		{
			name: "app_release",
			mace: `[output = data]
{
  app: "MaceBoard",
  version: "1.8.3",
  build: 184,
  stability_score: 98.6,
  production: true,
  maintainers: ["Ada", "Linus", "Grace"],
  features: [
    {
      name: "schema-output",
      enabled: true,
      rollout_percent: 100.0
    },
    {
      name: "data-output",
      enabled: true,
      rollout_percent: 87.5
    }
  ],
  metadata: {
    repository: "github.com/example/maceboard",
    license: "MIT",
    tags: ["config", "language", "tooling"]
  }
}`,
		},
		{
			name: "bookstore_order",
			mace: `[output = data]
{
  order_id: "ord-2026-0508-001",
  paid: true,
  item_count: 3,
  subtotal: 58.47,
  customer: {
    name: "Mira Chen",
    loyalty_points: 1280,
    newsletter: false
  },
  items: [
    {
      sku: "bk-parser-001",
      title: "Parsing By Candlelight",
      quantity: 1,
      price: 29.99
    },
    {
      sku: "bk-config-007",
      title: "Configuration Garden",
      quantity: 2,
      price: 14.24
    }
  ],
  shipping: {
    method: "ground",
    insured: true,
    address: {
      city: "Coram",
      state: "NY",
      postal_code: "11727"
    }
  }
}`,
		},
		{
			name: "deep_observatory_network",
			mace: `[output = data]
{
  network: {
    id: "obs-net-east",
    active: true,
    region_count: 3,
    average_uptime: 99.982,
    regions: [
      {
        name: "north-atlantic",
        priority: 1,
        stations: [
          {
            code: "NA-001",
            online: true,
            calibration: {
              version: "2026.05",
              drift: 0.002,
              instruments: [
                {
                  name: "spectrometer",
                  channels: 128,
                  thresholds: {
                    warning: 0.75,
                    critical: 0.92,
                    notify: true
                  }
                }
              ]
            }
          }
        ]
      }
    ],
    governance: {
      owner: {
        team: "Sky Ops",
        contacts: [
          {
            name: "Rhea",
            role: "operator",
            escalation: {
              primary: true,
              level: 2,
              windows: ["day", "night"]
            }
          }
        ]
      }
    }
  }
}`,
		},
		{
			name: "game_character",
			mace: `[output = data]
{
  name: "Nyra",
  class: "Spellblade",
  level: 42,
  health: 935.5,
  active: true,
  inventory: [
    {
      id: "blade-ember",
      quantity: 1,
      equipped: true
    },
    {
      id: "mana-vial",
      quantity: 6,
      equipped: false
    }
  ],
  stats: {
    strength: 18,
    agility: 24,
    intelligence: 31,
    critical_chance: 0.275
  },
  quests: [
    {
      title: "Glass Moon",
      completed: false,
      steps: ["Find shard", "Restore mirror", "Defeat warden"]
    }
  ]
}`,
		},
		{
			name: "smart_home",
			mace: `[output = data]
{
  home: "Cedar Loft",
  occupied: true,
  floor_count: 2,
  indoor_temperature: 70.8,
  rooms: [
    {
      name: "Kitchen",
      lights_on: true,
      humidity: 44.5,
      sensors: ["motion", "smoke", "temperature"]
    },
    {
      name: "Studio",
      lights_on: false,
      humidity: 39.2,
      sensors: ["motion", "temperature"]
    }
  ],
  automation: {
    away_mode: false,
    thermostat_target: 69,
    night_routine: {
      enabled: true,
      start_hour: 22,
      actions: ["lock doors", "dim lights", "lower thermostat"]
    }
  }
}`,
		},
	}
}

var _ = Describe("import conversion", func() {
	It("handles YAML scalar names and reports unsupported YAML name nodes", func() {
		fieldName := yamlFieldName
		fieldNameFromNode := yamlFieldNameFromNode
		anchorName := yamlAnchorName
		aliasName := yamlAliasName

		name, err := fieldName(&yamlast.StringNode{Value: "service"})
		tAssert.NoError(err)
		tAssert.Equal("service", name)

		name, err = fieldName(&yamlast.MappingKeyNode{Value: &yamlast.StringNode{Value: "mapped"}})
		tAssert.NoError(err)
		tAssert.Equal("mapped", name)

		_, err = fieldNameFromNode(&yamlast.IntegerNode{Value: int64(1)})
		tAssert.ErrorContains(err, "unsupported map key")

		name, err = anchorName(&yamlast.StringNode{Value: "defaults"})
		tAssert.NoError(err)
		tAssert.Equal("defaults", name)

		_, err = anchorName(&yamlast.IntegerNode{Value: int64(1)})
		tAssert.ErrorContains(err, "unsupported anchor name")

		name, err = aliasName(&yamlast.StringNode{Value: "defaults"})
		tAssert.NoError(err)
		tAssert.Equal("defaults", name)

		_, err = aliasName(&yamlast.IntegerNode{Value: int64(1)})
		tAssert.ErrorContains(err, "unsupported alias")
	})

	It("converts schema references to Mace paths", func() {
		referenceToMace := schemaReferenceToMace
		pathToMace := schemaPathToMace
		referenceParts := schemaReferenceParts
		relativePath := explicitRelativeSchemaPath

		tAssert.Equal("", referenceToMace(""))
		tAssert.Equal("schemas/config.mace", referenceToMace("schemas/config.json"))
		tAssert.Equal("https://example.test/schemas/config.mace?draft=1", referenceToMace("https://example.test/schemas/config.json?draft=1"))
		tAssert.Equal("schema.mace?version=1#/$defs/User", pathToMace("schema.json?version=1#/$defs/User", "/"))

		base, suffix := referenceParts("schema.json#/$defs/User")
		tAssert.Equal("schema.json", base)
		tAssert.Equal("#/$defs/User", suffix)

		base, suffix = referenceParts("schema.json?version=1")
		tAssert.Equal("schema.json", base)
		tAssert.Equal("?version=1", suffix)

		tAssert.Equal("", relativePath(""))
		tAssert.Equal("./schema.mace", relativePath("schema.mace"))
		tAssert.Equal("../schema.mace", relativePath("../schema.mace"))
	})

	It("handles reflected TOML fallback values", func() {
		reflectExpression := reflectTOMLExpression

		expression, err := reflectExpression(reflect.ValueOf(map[string]any{
			"name": "Ada",
			"tags": []any{"config", "tool"},
		}), nil, tomlImportConfig{})
		tAssert.NoError(err)
		tAssert.Equal(`{
  name: "Ada",
  tags: [
    "config",
    "tool"
  ]
}`, expression.render(0))

		expression, err = reflectExpression(reflect.ValueOf((*map[string]any)(nil)), nil, tomlImportConfig{})
		tAssert.NoError(err)
		tAssert.Equal(`""`, expression.render(0))

		expression, err = reflectExpression(reflect.ValueOf([]map[string]any{{"name": "Ada"}}), nil, tomlImportConfig{})
		tAssert.NoError(err)
		tAssert.Equal(`[
  {
    name: "Ada"
  }
]`, expression.render(0))

		_, err = reflectExpression(reflect.ValueOf(1), nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported value")
	})

	It("renders omitted import expressions as empty text", func() {
		render := omittedExpression{}.render

		tAssert.Equal("", render(0))
	})

	It("imports YAML data fixtures into equivalent Mace output", func() {
		for _, fixture := range dataFixtures() {
			expected := expectedOutput(fixture.mace)

			input, err := goccyyaml.Marshal(expected)
			tAssert.NoError(err)

			actualSource, err := importYAMLSource(fixture.name+".yaml", string(input))
			tAssert.NoError(err, fixture.name)
			tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(actualSource)), fixture.name)
		}
	})

	It("imports TOML data fixtures into equivalent Mace output", func() {
		for _, fixture := range dataFixtures() {
			expected := expectedOutput(fixture.mace)

			var buffer bytes.Buffer
			err := burnttoml.NewEncoder(&buffer).Encode(expected)
			tAssert.NoError(err)

			actualSource, err := importTOMLSource(fixture.name+".toml", buffer.String())
			tAssert.NoError(err, fixture.name)
			tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(actualSource)), fixture.name)
		}
	})

	It("imports the basic YAML alias fixture", func() {
		input := `defaults: &defaults
  retry_count: 3
  timeout_seconds: 30
  enabled: true

development:
  config: *defaults

production:
  config: *defaults
`

		source, err := importYAMLSource(filepath.Join("workspace", "01_basic_alias.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  defaults: {
    retry_count: 3,
    timeout_seconds: 30,
    enabled: true
  },
  development: {
    config: $self.defaults
  },
  production: {
    config: $self.defaults
  }
}`, source)

		expected := expectedOutput(`[output = data]
{
  defaults: {
    retry_count: 3,
    timeout_seconds: 30,
    enabled: true
  },
  development: {
    config: {
      retry_count: 3,
      timeout_seconds: 30,
      enabled: true
    }
  },
  production: {
    config: {
      retry_count: 3,
      timeout_seconds: 30,
      enabled: true
    }
  }
}`)
		tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(source)))
	})

	It("imports the YAML merge override fixture", func() {
		input := `base_service: &base_service
  image: mace/api
  replicas: 2
  port: 8080
  env:
    LOG_LEVEL: info
    CACHE_ENABLED: true

api_service:
  <<: *base_service
  replicas: 4
  env:
    LOG_LEVEL: debug
    CACHE_ENABLED: true
`

		source, err := importYAMLSource(filepath.Join("workspace", "02_merge_key_override.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  base_service: {
    image: "mace/api",
    replicas: 2,
    port: 8080,
    env: {
      LOG_LEVEL: "info",
      CACHE_ENABLED: true
    }
  },
  api_service: base_service <> {
    replicas: 4,
    env: {
      LOG_LEVEL: "debug",
      CACHE_ENABLED: true
    }
  }
}`, source)

		expected := expectedOutput(`[output = data]
{
  base_service: {
    image: "mace/api",
    replicas: 2,
    port: 8080,
    env: {
      LOG_LEVEL: "info",
      CACHE_ENABLED: true
    }
  },
  api_service: {
    image: "mace/api",
    replicas: 4,
    port: 8080,
    env: {
      LOG_LEVEL: "debug",
      CACHE_ENABLED: true
    }
  }
}`)
		tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(source)))
	})

	It("imports the nested YAML anchor fixture", func() {
		input := `database_defaults: &database_defaults
  host: db.internal
  port: 5432
  credentials: &database_credentials
    username: mace_app
    password_ref: secret/database/password

services:
  writer:
    database:
      <<: *database_defaults
      credentials: *database_credentials
      pool_size: 20
  reader:
    database:
      <<: *database_defaults
      credentials: *database_credentials
      pool_size: 8
`

		source, err := importYAMLSource(filepath.Join("workspace", "03_nested_anchor_alias.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  database_credentials: {
    username: "mace_app",
    password_ref: "secret/database/password"
  },
  database_defaults: {
    host: "db.internal",
    port: 5432,
    credentials: $self.database_credentials
  },
  services: {
    writer: {
      database: database_defaults <> {
        credentials: $self.database_credentials,
        pool_size: 20
      }
    },
    reader: {
      database: database_defaults <> {
        credentials: $self.database_credentials,
        pool_size: 8
      }
    }
  }
}`, source)

		expected := expectedOutput(`[output = data]
{
  database_credentials: {
    username: "mace_app",
    password_ref: "secret/database/password"
  },
  database_defaults: {
    host: "db.internal",
    port: 5432,
    credentials: $self.database_credentials
  },
  services: {
    writer: {
      database: database_defaults <> {
        credentials: $self.database_credentials,
        pool_size: 20
      }
    },
    reader: {
      database: database_defaults <> {
        credentials: $self.database_credentials,
        pool_size: 8
      }
    }
  }
}`)
		tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(source)))
	})

	It("imports the multi-source YAML merge fixture", func() {
		input := `runtime_defaults: &runtime_defaults
  restart: always
  memory_mb: 512

network_defaults: &network_defaults
  network: private
  expose_metrics: true

worker:
  <<:
    - *runtime_defaults
    - *network_defaults
  name: queue-worker
  memory_mb: 1024
`

		source, err := importYAMLSource(filepath.Join("workspace", "04_merge_multiple_sources.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  runtime_defaults: {
    restart: "always",
    memory_mb: 512
  },
  network_defaults: {
    network: "private",
    expose_metrics: true
  },
  worker: runtime_defaults <> network_defaults <> {
    name: "queue-worker",
    memory_mb: 1024
  }
}`, source)

		expected := expectedOutput(`[output = data]
{
  runtime_defaults: {
    restart: "always",
    memory_mb: 512
  },
  network_defaults: {
    network: "private",
    expose_metrics: true
  },
  worker: runtime_defaults <> network_defaults <> {
    name: "queue-worker",
    memory_mb: 1024
  }
}`)
		tAssert.Equal(canonicalJSON(expected), canonicalJSON(importedOutput(source)))
	})

	It("imports the deep nested YAML merge fixture", func() {
		input := `global_metadata: &global_metadata
  owner: platform-team
  labels: &global_labels
    app: mace
    tier: backend

base_container: &base_container
  image: mace/processor
  resources: &base_resources
    cpu: 2
    memory_gb: 4
  metadata:
    <<: *global_metadata
    labels:
      <<: *global_labels
      component: processor

deployment:
  regions:
    us_east:
      primary:
        <<: *base_container
        replicas: 3
        resources:
          <<: *base_resources
          memory_gb: 8
        metadata:
          <<: *global_metadata
          labels:
            <<: *global_labels
            component: processor
            region: us-east
      canary:
        <<: *base_container
        replicas: 1
        metadata:
          <<: *global_metadata
          labels:
            <<: *global_labels
            component: processor-canary
            region: us-east
    eu_west:
      primary:
        <<: *base_container
        replicas: 2
        metadata:
          <<: *global_metadata
          labels:
            <<: *global_labels
            component: processor
            region: eu-west
`

		source, err := importYAMLSource(filepath.Join("workspace", "05_deep_nested_merges.yaml"), input)
		tAssert.NoError(err)
		tAssert.Contains(source, "global_metadata <>")
		tAssert.Contains(source, "$self.global_labels")
		tAssert.Contains(source, "base_container <>")
		tAssert.Contains(source, "$self.base_resources")
		tAssert.GreaterOrEqual(strings.Count(source, "<>"), 6)

		output := importedOutput(source)
		deployment := output["deployment"].(map[string]any)
		regions := deployment["regions"].(map[string]any)
		usEast := regions["us_east"].(map[string]any)
		primary := usEast["primary"].(map[string]any)
		canary := usEast["canary"].(map[string]any)
		euWest := regions["eu_west"].(map[string]any)
		euPrimary := euWest["primary"].(map[string]any)

		tAssert.Equal("mace/processor", primary["image"])
		tAssert.Equal(int64(3), primary["replicas"])
		tAssert.Equal(int64(8), primary["resources"].(map[string]any)["memory_gb"])
		tAssert.Equal("us-east", primary["metadata"].(map[string]any)["labels"].(map[string]any)["region"])
		tAssert.Equal("processor-canary", canary["metadata"].(map[string]any)["labels"].(map[string]any)["component"])
		tAssert.Equal(int64(2), euPrimary["replicas"])
		tAssert.Equal("eu-west", euPrimary["metadata"].(map[string]any)["labels"].(map[string]any)["region"])
	})

	It("imports the game inventory YAML documents fixture", func() {
		input := `---
kind: game_inventory
player:
  id: player_42
  display_name: Moss Knight
  level: 17
inventory:
  weapons:
    - name: Thorn Blade
      damage: 42
      equipped: true
    - name: Moon Bow
      damage: 31
      equipped: false
  materials:
    wood: 120
    crystal: 9
    slime_gel: 44
---
kind: crafting_snapshot
recipes_unlocked:
  - healing_potion
  - shadow_lantern
can_craft:
  healing_potion: true
  shadow_lantern: false
`

		source, err := importYAMLSource(filepath.Join("workspace", "game_inventory.documents.yaml"), input)
		tAssert.NoError(err)
		tAssert.Contains(source, "document_1")
		tAssert.Contains(source, "document_2")

		output := importedOutput(source)
		document1 := output["document_1"].(map[string]any)
		document2 := output["document_2"].(map[string]any)
		weapons := document1["inventory"].(map[string]any)["weapons"].([]any)

		tAssert.Equal("game_inventory", document1["kind"])
		tAssert.Equal("Moss Knight", document1["player"].(map[string]any)["display_name"])
		tAssert.Len(weapons, 2)
		tAssert.Equal("crafting_snapshot", document2["kind"])
		tAssert.Equal([]any{"healing_potion", "shadow_lantern"}, document2["recipes_unlocked"])
	})

	It("imports the user catalog YAML documents fixture", func() {
		input := `---
kind: user_catalog
version: 1
users:
  - id: usr_001
    name: Ada Lovelace
    active: true
    score: 98.75
    roles:
      - admin
      - analyst
    profile:
      region: eu-west
      preferences:
        theme: dark
        notifications: true
---
kind: user_catalog_summary
version: 1
total_users: 1
primary_user:
  id: usr_001
  name: Ada Lovelace
`

		source, err := importYAMLSource(filepath.Join("workspace", "user_catalog.documents.yaml"), input)
		tAssert.NoError(err)
		tAssert.Contains(source, "document_1")
		tAssert.Contains(source, "document_2")

		output := importedOutput(source)
		document1 := output["document_1"].(map[string]any)
		document2 := output["document_2"].(map[string]any)
		users := document1["users"].([]any)

		tAssert.Equal(int64(1), document1["version"])
		tAssert.Len(users, 1)
		tAssert.Equal("Ada Lovelace", users[0].(map[string]any)["name"])
		tAssert.Equal("user_catalog_summary", document2["kind"])
		tAssert.Equal(int64(1), document2["total_users"])
	})

	It("converts YAML merge keys into Mace merge expressions", func() {
		input := `defaults: &defaults
  enabled: true
  retries: 3
profile:
  <<: *defaults
  retries: 5
  name: api
`

		source, err := importYAMLSource(filepath.Join("workspace", "merge_expression.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  defaults: {
    enabled: true,
    retries: 3
  },
  profile: defaults <> {
    retries: 5,
    name: "api"
  }
}`, source)

		output := importedOutput(source)
		profile := output["profile"].(map[string]any)
		tAssert.Equal(true, profile["enabled"])
		tAssert.Equal(int64(5), profile["retries"])
		tAssert.Equal("api", profile["name"])
	})

	It("keeps root YAML merge mappings at the top level", func() {
		input := `defaults: &defaults
  enabled: true
  retries: 3
<<: *defaults
name: api
`

		source, err := importYAMLSource(filepath.Join("workspace", "root_merge.yaml"), input)
		tAssert.NoError(err)
		tAssert.NotContains(source, "document_1")
		tAssert.Contains(source, "enabled: true")
		tAssert.Contains(source, "retries: 3")
		tAssert.Contains(source, "defaults: {")

		output := importedOutput(source)
		tAssert.Equal(true, output["enabled"])
		tAssert.Equal(int64(3), output["retries"])
		tAssert.Equal("api", output["name"])
	})

	It("orders hoisted YAML anchors before fields that depend on them", func() {
		input := `service:
  value_holder: &z
    host: db.internal
  alias_holder: &a
    target: *z
copy_a: *a
`

		source, err := importYAMLSource(filepath.Join("workspace", "anchor_dependency.yaml"), input)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  z: {
    host: "db.internal"
  },
  a: {
    target: $self.z
  },
  service: {
    value_holder: $self.z,
    alias_holder: $self.a
  },
  copy_a: ($self.a)
}`, source)

		output := importedOutput(source)
		copyA := output["copy_a"].(map[string]any)
		tAssert.Equal("db.internal", copyA["target"].(map[string]any)["host"])
	})

	It("normalizes YAML booleans and floats to valid Mace literals", func() {
		input := `enabled: TRUE
threshold: .5
`

		source, err := importYAMLSource(filepath.Join("workspace", "normalized_scalars.yaml"), input)
		tAssert.NoError(err)
		tAssert.Contains(source, "enabled: true")
		tAssert.Contains(source, "threshold: 0.5")

		output := importedOutput(source)
		tAssert.Equal(true, output["enabled"])
		tAssert.Equal(0.5, output["threshold"])
	})

	It("omits YAML null values instead of converting them to empty strings", func() {
		input := `name: Ada
nickname: null
tags:
  - alpha
  - null
  - beta
`

		source, err := importYAMLSource(filepath.Join("workspace", "null_omission.yaml"), input)
		tAssert.NoError(err)
		tAssert.NotContains(source, "nickname:")
		tAssert.NotContains(source, "\"\"")

		output := importedOutput(source)
		tAssert.Equal("Ada", output["name"])
		_, hasNickname := output["nickname"]
		tAssert.False(hasNickname)
		tAssert.Equal([]any{"alpha", "beta"}, output["tags"])
	})

	It("imports TOML schema directives, tables, inline tables, arrays of tables, dotted keys, and multiline strings", func() {
		input := `#:schema ./schemas/vehicle_telemetry.schema.json
name = "orbital-array"
enabled = true
score = 42.5
tags = ["edge", "night"]
description = """
Line one
Line two
"""

metrics.cpu = 0.25
metrics.mem = 0.75

[owner]
name = "Ada"
active = true

[[sensors]]
id = "sensor-a"
kind = "temp"

[[sensors]]
id = "sensor-b"
kind = "pressure"

[location]
point = { lat = 51.5, lon = -0.1 }
updated_at = 2026-05-08T09:00:00Z
`

		source, err := importTOMLSource(filepath.Join("workspace", "config.toml"), input)
		tAssert.NoError(err)
		tAssert.Contains(source, `[output = data, schema_file = "./schemas/vehicle_telemetry.schema.mace"]`)
		tAssert.Contains(source, "description: \"\"\"")

		output := importedOutput(strings.Replace(source, `, schema_file = "./schemas/vehicle_telemetry.schema.mace"`, "", 1))
		tAssert.Equal("orbital-array", output["name"])
		tAssert.Equal(true, output["enabled"])
		tAssert.Equal(42.5, output["score"])
		tAssert.Equal([]any{"edge", "night"}, output["tags"])
		tAssert.Equal("Line one\nLine two\n", output["description"])
		tAssert.Equal(map[string]any{"name": "Ada", "active": true}, output["owner"])
		tAssert.Equal(map[string]any{"cpu": 0.25, "mem": 0.75}, output["metrics"])
		sensors := output["sensors"].([]any)
		tAssert.Len(sensors, 2)
		tAssert.Equal(map[string]any{"id": "sensor-a", "kind": "temp"}, sensors[0])
		tAssert.Equal(map[string]any{"id": "sensor-b", "kind": "pressure"}, sensors[1])
		location := output["location"].(map[string]any)
		tAssert.Equal(map[string]any{"lat": 51.5, "lon": -0.1}, location["point"])
		tAssert.Equal(time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano), location["updated_at"])
	})

	It("rebases schema directives when imports are written to an output directory", func() {
		input := `#:schema ./schemas/vehicle_telemetry.schema.json
name = "orbital-array"
`

		source, err := importTOMLSourceToPath(
			filepath.Join("workspace", "config.toml"),
			filepath.Join("out", "config.mace"),
			input,
		)
		tAssert.NoError(err)
		tAssert.Contains(source, `[output = data, schema_file = "../workspace/schemas/vehicle_telemetry.schema.mace"]`)
	})

	It("covers import conversion edge cases", func() {
		tAssert.NoError(validateImportFieldName("valid_name"))
		tAssert.ErrorContains(validateImportFieldName("invalid-name"), "unsupported field name")

		tAssert.Equal("$self.name", selfFieldPath("", "name"))
		tAssert.Equal("$self.items[2]", selfIndexPath("$self.items", 2))
		tAssert.Equal("", selfIndexPath("", 2))
		tAssert.Equal([]string{"[3]", "[4]"}, appendIndex([]string{"[3]"}, 4))
		tAssert.Equal("group.member", pathKey([]string{"group", "member"}))
		tAssert.Equal(`"""quoted"""`, tripleQuotedString("quoted"))
		tAssert.True(isOmittedImportExpression(omittedExpression{}))
		tAssert.False(isOmittedImportExpression(rawExpression{text: "value"}))

		emptyDocs := yamlast.File{}
		_, err := yamlRootExpression(&emptyDocs)
		tAssert.ErrorContains(err, "expected at least one document")

		scalarSource, err := importYAMLSource("workspace/scalar.yaml", "hello")
		tAssert.NoError(err)
		tAssert.Contains(scalarSource, "document_1")

		tagSource, err := importYAMLSource("workspace/tag.yaml", "!foo hello")
		tAssert.NoError(err)
		tAssert.Contains(tagSource, "hello")

		multiDocSource, err := importYAMLSource("workspace/multi.yaml", "---\nhello\n---\nworld\n")
		tAssert.NoError(err)
		tAssert.Contains(multiDocSource, "document_1")
		tAssert.Contains(multiDocSource, "document_2")

		infinitySource, err := importYAMLSource("workspace/infinity.yaml", "value: .inf")
		tAssert.NoError(err)
		tAssert.Contains(infinitySource, `".inf"`)

		nanSource, err := importYAMLSource("workspace/nan.yaml", "value: .nan")
		tAssert.NoError(err)
		tAssert.Contains(nanSource, `".nan"`)

		literalSource, err := importYAMLSource("workspace/literal.yaml", "value: |\n  line one\n  line two\n")
		tAssert.NoError(err)
		tAssert.Contains(literalSource, `"""`)

		_, err = importYAMLSourceToPath("workspace/invalid.yaml", "workspace/out.mace", "foo-bar: 1")
		tAssert.ErrorContains(err, "unsupported field name")

		_, err = yamlFieldName(&yamlast.IntegerNode{Value: 1})
		tAssert.ErrorContains(err, "unsupported map key")

		_, err = importYAMLSourceToPath("workspace/parse.yaml", "workspace/out.mace", "[")
		tAssert.Error(err)

		_, err = importYAMLSourceToPath("workspace/merge.yaml", "workspace/out.mace", "<<: *missing")
		tAssert.Error(err)

		_, err = importYAMLSourceToPath("workspace/merge-invalid.yaml", "workspace/out.mace", "<<: value")
		tAssert.ErrorContains(err, "merge source")

		_, err = importYAMLSourceToPath("workspace/multi-merge.yaml", "workspace/out.mace", "---\n<<: value\n---\nname: Ada\n")
		tAssert.Error(err)

		_, err = importYAMLSourceToPath("workspace/anchor.yaml", "workspace/out.mace", "&defaults 1")
		tAssert.Error(err)

		_, err = importYAMLSourceToPath("workspace/alias.yaml", "workspace/out.mace", "value: *missing")
		tAssert.ErrorContains(err, "unknown alias")

		_, err = importTOMLSourceToPath("workspace/invalid.toml", "workspace/out.mace", "name =")
		tAssert.Error(err)

		_, err = importTOMLSourceToPath("workspace/invalid-field.toml", "workspace/out.mace", "bad-name = 1")
		tAssert.Error(err)

		stringerExpression, err := tomlExpression(quotedStringer("hello"), nil, tomlImportConfig{})
		tAssert.NoError(err)
		tAssert.Equal(`"hello"`, stringerExpression.render(0))

		_, err = tomlExpression(nil, nil, tomlImportConfig{})
		tAssert.NoError(err)

		_, err = tomlExpression(uint(7), nil, tomlImportConfig{})
		tAssert.NoError(err)

		_, err = tomlExpression(float32(1.5), nil, tomlImportConfig{})
		tAssert.NoError(err)

		_, err = tomlExpression(map[string]any{"bad-name": 1}, nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported field name")

		_, err = tomlExpression([]any{struct{}{}}, nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported value")

		_, err = tomlExpression([]map[string]any{{"bad-name": 1}}, nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported field name")

		_, err = tomlRecordExpression(map[string]any{"nested": []any{struct{}{}}}, nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported value")

		_, err = reflectTOMLExpression(reflect.Value{}, nil, tomlImportConfig{})
		tAssert.NoError(err)

		value := 7
		_, err = reflectTOMLExpression(reflect.ValueOf(&value), nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported value int")

		_, err = reflectTOMLExpression(reflect.ValueOf([]any{struct{}{}}), nil, tomlImportConfig{})
		tAssert.ErrorContains(err, "unsupported value")

		basePath, err := filepath.Abs("schema.mace")
		tAssert.NoError(err)
		tAssert.Equal(basePath, explicitRelativeSchemaPath(basePath))

		related := adjustedSchemaReferenceToMace("schemas/config.json", filepath.Join("workspace", "input.yaml"), filepath.Join("workspace", "output.mace"))
		tAssert.Equal("./schemas/config.mace", related)

		absolutePath, err := filepath.Abs("schemas/config.json")
		tAssert.NoError(err)
		tAssert.Contains(adjustedSchemaReferenceToMace(absolutePath, filepath.Join("workspace", "input.yaml"), ""), "config.mace")
		tAssert.Contains(adjustedSchemaReferenceToMace("https://example.com/schema.json", filepath.Join("workspace", "input.yaml"), ""), "https://example.com")
		tAssert.Equal(".mace#fragment", adjustedSchemaReferenceToMace("#fragment", filepath.Join("workspace", "input.yaml"), ""))

		record := recordExpression{fields: []recordField{
			{name: "left", value: rawExpression{text: "$self.right"}},
			{name: "right", value: rawExpression{text: "1"}},
		}}
		ordered, err := yamlOrderedFieldNames([]string{"left", "right"}, map[string]recordField{
			"left":  record.fields[0],
			"right": record.fields[1],
		})
		tAssert.NoError(err)
		tAssert.Equal([]string{"right", "left"}, ordered)

		_, err = yamlOrderedFieldNames([]string{"left", "right"}, map[string]recordField{
			"left":  {name: "left", value: rawExpression{text: "$self.right"}},
			"right": {name: "right", value: rawExpression{text: "$self.left"}},
		})
		tAssert.ErrorContains(err, "cyclic")

		_, ok := yamlTopLevelReferenceName("$self.name")
		tAssert.True(ok)
		_, ok = yamlTopLevelReferenceName("$self.name[0]")
		tAssert.False(ok)

		yamlState := &yamlImportState{
			anchors: map[string]yamlAnchor{
				"defaults": {
					path:  "$self.defaults",
					value: recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: `"Ada"`}}}},
				},
			},
			hoists: map[string]importExpression{},
		}

		nilNode, err := yamlNodeExpression(nil, "", yamlState)
		tAssert.NoError(err)
		tAssert.True(isOmittedImportExpression(nilNode))

		documentNode, err := yamlNodeExpression(&yamlast.DocumentNode{Body: &yamlast.StringNode{Value: "doc"}}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Equal(`"doc"`, documentNode.render(0))

		tagNode, err := yamlNodeExpression(&yamlast.TagNode{Value: &yamlast.StringNode{Value: "tag"}}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Equal(`"tag"`, tagNode.render(0))

		sequenceNode, err := yamlNodeExpression(&yamlast.SequenceNode{Values: []yamlast.Node{&yamlast.StringNode{Value: "item"}}}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Equal("[\n  \"item\"\n]", sequenceNode.render(0))

		_, err = yamlNodeExpression(&yamlast.SequenceNode{Values: []yamlast.Node{&yamlast.AnchorNode{Name: &yamlast.IntegerNode{Value: 1}, Value: &yamlast.StringNode{Value: "item"}}}}, "", yamlState)
		tAssert.Error(err)

		nullNode, err := yamlNodeExpression(&yamlast.NullNode{}, "", yamlState)
		tAssert.NoError(err)
		tAssert.True(isOmittedImportExpression(nullNode))

		_, err = yamlNodeExpression(&yamlast.AnchorNode{Name: &yamlast.IntegerNode{Value: 1}, Value: &yamlast.StringNode{Value: "value"}}, "", yamlState)
		tAssert.ErrorContains(err, "unsupported anchor name")

		_, err = yamlNodeExpression(&yamlast.AnchorNode{Name: &yamlast.StringNode{Value: "defaults"}, Value: &yamlast.AliasNode{Value: &yamlast.StringNode{Value: "missing"}}}, "$self.service", yamlState)
		tAssert.ErrorContains(err, "unknown alias")

		_, err = yamlNodeExpression(&yamlast.AliasNode{Value: &yamlast.IntegerNode{Value: 1}}, "", yamlState)
		tAssert.ErrorContains(err, "unsupported alias")

		aliasState := &yamlImportState{anchors: map[string]yamlAnchor{"defaults": {path: "$self.defaults", value: omittedExpression{}}}, hoists: map[string]importExpression{}}
		aliasNode, err := yamlNodeExpression(&yamlast.AliasNode{Value: &yamlast.StringNode{Value: "defaults"}}, "", aliasState)
		tAssert.NoError(err)
		tAssert.True(isOmittedImportExpression(aliasNode))

		mappingValueNode, err := yamlNodeExpression(&yamlast.MappingValueNode{Key: &yamlast.StringNode{Value: "name"}, Value: &yamlast.StringNode{Value: "Ada"}}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Contains(mappingValueNode.render(0), "name")

		_, err = yamlNodeExpression(&yamlast.MappingValueNode{Key: &yamlast.StringNode{Value: "bad name"}, Value: &yamlast.StringNode{Value: "Ada"}}, "", yamlState)
		tAssert.ErrorContains(err, "unsupported field name")

		_, err = yamlNodeExpression(&yamlast.MappingValueNode{Key: &yamlast.IntegerNode{Value: 1}, Value: &yamlast.StringNode{Value: "Ada"}}, "", yamlState)
		tAssert.ErrorContains(err, "unsupported map key")

		_, err = yamlNodeExpression(&yamlast.DirectiveNode{}, "", yamlState)
		tAssert.Error(err)

		_, err = yamlNodeExpression(&yamlast.AnchorNode{Name: &yamlast.StringNode{Value: "defaults"}, Value: &yamlast.StringNode{Value: "value"}}, "", yamlState)
		tAssert.ErrorContains(err, "must be attached")

		_, err = yamlNodeExpression(&yamlast.AliasNode{Value: &yamlast.StringNode{Value: "missing"}}, "", yamlState)
		tAssert.ErrorContains(err, "unknown alias")

		yamlState = &yamlImportState{
			anchors: map[string]yamlAnchor{
				"defaults": {
					path:  "$self.defaults",
					value: recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: `"Ada"`}}}},
				},
			},
			hoists: map[string]importExpression{},
		}

		resolved, err := yamlResolvedRecord(rawExpression{text: "$self.defaults"}, yamlState)
		tAssert.NoError(err)
		tAssert.Len(resolved.fields, 1)

		mergeResolved, err := yamlResolvedRecord(mergeExpression{parts: []importExpression{rawExpression{text: "$self.defaults"}}}, yamlState)
		tAssert.NoError(err)
		tAssert.Len(mergeResolved.fields, 1)

		_, err = yamlResolvedRecord(mergeExpression{parts: []importExpression{rawExpression{text: "value"}}}, yamlState)
		tAssert.ErrorContains(err, "not a record")

		duplicated, err := yamlResolvedRecord(mergeExpression{parts: []importExpression{
			recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: "1"}}}},
			recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: "2"}}}},
		}}, yamlState)
		tAssert.NoError(err)
		tAssert.Equal("2", duplicated.fields[0].value.render(0))

		_, err = yamlResolvedRecord(rawExpression{text: "value"}, yamlState)
		tAssert.ErrorContains(err, "not a record")

		_, err = yamlResolvedRecord(rawExpression{text: "$self.missing"}, yamlState)
		tAssert.ErrorContains(err, "unknown merge source")

		_, err = yamlResolvedRecord(omittedExpression{}, yamlState)
		tAssert.Error(err)

		merged, err := yamlResolvedRecord(mergeExpression{parts: []importExpression{recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: `"Ada"`}}}}}}, yamlState)
		tAssert.NoError(err)
		tAssert.Len(merged.fields, 1)

		dependencies := yamlExpressionDependencies(recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: "$self.defaults"}}}}, map[string]recordField{"defaults": {name: "defaults", value: rawExpression{text: `"Ada"`}}})
		tAssert.Contains(dependencies, "defaults")

		nestedDependencies := yamlExpressionDependencies(arrayExpression{items: []importExpression{
			recordExpression{fields: []recordField{{name: "inner", value: rawExpression{text: "$self.defaults"}}}},
			mergeExpression{parts: []importExpression{rawExpression{text: "$self.defaults"}}},
		}}, map[string]recordField{"defaults": {name: "defaults", value: rawExpression{text: `"Ada"`}}})
		tAssert.Contains(nestedDependencies, "defaults")

		hoistState := &yamlImportState{
			hoists: map[string]importExpression{
				"existing": rawExpression{text: "1"},
				"hoisted":  rawExpression{text: `"Ada"`},
				"skipped":  omittedExpression{},
			},
			hoistOrder: []string{"existing", "hoisted", "skipped"},
		}
		hoistedRecord, err := yamlRecordWithHoists(recordExpression{fields: []recordField{{name: "existing", value: rawExpression{text: "2"}}}}, hoistState)
		tAssert.NoError(err)
		tAssert.Len(hoistedRecord.fields, 2)

		emptyHoistsRecord, err := yamlRecordWithHoists(recordExpression{}, &yamlImportState{hoists: map[string]importExpression{}})
		tAssert.NoError(err)
		tAssert.NoError(err)
		tAssert.Empty(emptyHoistsRecord.fields)

		missingHoistRecord, err := yamlRecordWithHoists(recordExpression{}, &yamlImportState{
			hoists:     map[string]importExpression{"present": rawExpression{text: "1"}},
			hoistOrder: []string{"missing", "present"},
		})
		tAssert.NoError(err)
		tAssert.Len(missingHoistRecord.fields, 1)

		rootRecord, err := yamlRootExpression(&yamlast.File{Docs: []*yamlast.DocumentNode{{Body: &yamlast.AnchorNode{Name: &yamlast.StringNode{Value: "defaults"}, Value: &yamlast.StringNode{Value: "value"}}}}})
		tAssert.Error(err)
		tAssert.Empty(rootRecord.fields)

		multiErrorRoot, err := yamlRootExpression(&yamlast.File{Docs: []*yamlast.DocumentNode{{Body: &yamlast.AnchorNode{Name: &yamlast.StringNode{Value: "defaults"}, Value: &yamlast.AliasNode{Value: &yamlast.StringNode{Value: "missing"}}}}, {Body: &yamlast.StringNode{Value: "hello"}}}})
		tAssert.Error(err)
		tAssert.Empty(multiErrorRoot.fields)

		multiRoot, err := yamlRootExpression(&yamlast.File{Docs: []*yamlast.DocumentNode{{Body: &yamlast.StringNode{Value: "hello"}}, {Body: &yamlast.StringNode{Value: "world"}}}})
		tAssert.NoError(err)
		tAssert.Len(multiRoot.fields, 2)

		_, ok, err = yamlDocumentRecord(recordExpression{}, yamlState)
		tAssert.NoError(err)
		tAssert.True(ok)

		_, ok, err = yamlDocumentRecord(rawExpression{text: "value"}, yamlState)
		tAssert.NoError(err)
		tAssert.False(ok)

		_, ok, err = yamlDocumentRecord(mergeExpression{parts: []importExpression{recordExpression{fields: []recordField{{name: "name", value: rawExpression{text: `"Ada"`}}}}}}, yamlState)
		tAssert.NoError(err)
		tAssert.True(ok)

		documentOrder := orderedRecordNames(map[string]any{"b": 1, "a": 2}, nil, map[string][]string{"": {"missing", "b"}})
		tAssert.Equal([]string{"b", "a"}, documentOrder)

		tAssert.Equal("[]", arrayExpression{}.render(0))
		tAssert.Equal("{}", recordExpression{}.render(0))

		parts, err := yamlMergeExpressions(&yamlast.TagNode{Value: &yamlast.SequenceNode{Values: []yamlast.Node{&yamlast.StringNode{Value: "item"}}}}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Len(parts, 1)

		_, err = yamlMergeExpressions(&yamlast.SequenceNode{Values: []yamlast.Node{&yamlast.AnchorNode{Name: &yamlast.IntegerNode{Value: 1}, Value: &yamlast.StringNode{Value: "item"}}}}, "", yamlState)
		tAssert.Error(err)

		singlePart, err := yamlMergeExpressions(&yamlast.StringNode{Value: "item"}, "", yamlState)
		tAssert.NoError(err)
		tAssert.Len(singlePart, 1)

		_, err = formatImportedSource("$x")
		tAssert.Error(err)

		_, err = formatImportedSource("[output = data]\n{")
		tAssert.Error(err)

		_, err = formatImportedSource("[output = data]\n$")
		tAssert.Error(err)

		previous := formatMaceFile
		defer func() { formatMaceFile = previous }()
		formatMaceFile = func(ast.File) (string, error) {
			return "", errors.New("format failed")
		}

		_, err = formatImportedSource("[output = data]\n{ name: \"Ada\", }")
		tAssert.ErrorContains(err, "format generated source")
		tAssert.Error(err)

		_, err = importSourceFromPath(filepath.Join("workspace", "missing.yaml"), "")
		tAssert.Error(err)

		badOutputDir := writeTempFile("output-dir", "")
		_, err = importOutputPath("config.json", badOutputDir)
		tAssert.Error(err)

		tAssert.Equal("", adjustedSchemaPath("workspace/input.yaml", "workspace/output.mace", "name = 1", tomlSchemaPattern))
		tAssert.Equal("", adjustedSchemaReferenceToMace("   ", "workspace/input.yaml", "workspace/output.mace"))
	})
})
