package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type OpenAIQuotaPeriod struct{ ent.Schema }

func (OpenAIQuotaPeriod) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "openai_quota_periods"}}
}

func (OpenAIQuotaPeriod) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Time("started_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("ended_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("reset_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("request_count").Default(0),
		field.Int64("token_count").Optional().Nillable(),
		field.Float("used_usd").Default(0).SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.Float("used_percent").Default(0).SchemaType(map[string]string{dialect.Postgres: "numeric(8,4)"}),
		field.Float("predicted_quota_usd").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "numeric(20,8)"}),
		field.Time("snapshot_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (OpenAIQuotaPeriod) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "started_at").Unique(),
	}
}
