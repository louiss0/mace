package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	burnttoml "github.com/BurntSushi/toml"
	goccyyaml "github.com/goccy/go-yaml"
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
  copy_a: $self.a
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
})
