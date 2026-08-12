package dbctx_test

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/shrsv/dbctx"
)

// This example demonstrates building an in-memory index from PostgreSQL
// and querying it with natural language. The output is a compact,
// notation-annotated schema ready for an LLM or text-to-SQL system.
func Example() {
	ctx := context.Background()

	idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	result, err := idx.Query("failed reviews last month")
	if err != nil {
		log.Fatal(err)
	}

	// Matched() returns tables that scored a direct hit plus FK-expanded
	// join context — everything needed to answer the query.
	// Text() prepends a notation legend explaining every symbol.
	fmt.Println(result.Matched().Text())
}

// This example demonstrates saving an index to a .dtx file and opening
// it later without a PostgreSQL connection.
func Example_persist() {
	ctx := context.Background()
	dsn := "postgres://localhost/mydb"

	// Build and save to disk.
	idx, err := dbctx.Build(ctx, dsn, &dbctx.Options{Path: "mydb.dtx"})
	if err != nil {
		log.Fatal(err)
	}
	idx.Close()

	// Reopen without PostgreSQL.
	idx, err = dbctx.Open("mydb.dtx")
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	tables, _ := idx.Tables()
	fmt.Printf("Index has %d tables\n", len(tables))
}

// This example demonstrates non-blocking startup with BuildAsync.
// The index builds in a background goroutine while the application
// continues setup. Queries block automatically until the index is ready.
func Example_buildAsync() {
	ctx := context.Background()
	dsn := "postgres://localhost/mydb"

	idx, ready, err := dbctx.BuildAsync(ctx, dsn, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	// Register idx with your application immediately.
	go func() {
		<-ready
		log.Println("dbctx index is ready")
	}()

	// This call blocks automatically if the index isn't ready yet.
	result, err := idx.Query("active users")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Matched().Text())
}

// This example demonstrates the Selection API for filtering and rendering
// query results in different ways.
func Example_selection() {
	ctx := context.Background()

	idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	result, err := idx.Query("failed reviews")
	if err != nil {
		log.Fatal(err)
	}

	// Matched + FK-expanded tables, with notation legend.
	fmt.Println(result.Matched().Text())

	// Without legend (tighter token budget).
	fmt.Println(result.Matched().TextRaw())

	// Only tables that scored directly, excluding FK-expanded join context.
	fmt.Println(result.ScoredOnly().TextRaw())

	// Specific tables by name.
	fmt.Println(result.Include("reviews", "orgs").TextRaw())

	// Matched minus a table.
	fmt.Println(result.Matched().Exclude("migrations").TextRaw())
}

// This example demonstrates opening a persisted .dtx file and listing
// all tables with summary information.
func ExampleOpen() {
	idx, err := dbctx.Open("mydb.dtx")
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	tables, err := idx.Tables()
	if err != nil {
		log.Fatal(err)
	}

	for _, t := range tables {
		fmt.Printf("%-30s %6.0f rows  %d cols  %d FKs\n",
			t.Name, t.RowEstimate, t.ColCount, t.FKCount)
	}
}

// This example demonstrates getting detailed information about a single
// table including columns, types, primary keys, foreign keys, and
// representative values for state-like fields.
func ExampleIndex_TableDetail() {
	ctx := context.Background()

	idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	detail, err := idx.TableDetail("reviews")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Table: %s\n", detail.Name)
	fmt.Printf("Primary key: %s\n", strings.Join(detail.PrimaryKey, ", "))
	fmt.Printf("Columns: %d\n", len(detail.Columns))

	for _, col := range detail.Columns {
		flags := ""
		if col.IsPK {
			flags += " PK"
		}
		if col.IsState {
			flags += " [state]"
		}
		if col.FKTarget != "" {
			flags += " -> " + col.FKTarget
		}
		fmt.Printf("  %-20s %-20s%s\n", col.Name, col.Type, flags)
	}
}

// This example demonstrates getting summary statistics about the index.
func ExampleIndex_Stats() {
	ctx := context.Background()

	idx, err := dbctx.Build(ctx, "postgres://localhost/mydb", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer idx.Close()

	stats, err := idx.Stats()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Tables:            %d\n", stats.Tables)
	fmt.Printf("Columns:           %d\n", stats.Columns)
	fmt.Printf("Foreign keys:      %d\n", stats.ForeignKeys)
	fmt.Printf("State fields:      %d\n", stats.StateFields)
	fmt.Printf("Categorical fields: %d\n", stats.CategoricalFields)
	fmt.Printf("JSONB paths:       %d\n", stats.JSONBPaths)
}
