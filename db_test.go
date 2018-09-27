package cypress

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

type member struct {
	id        int32
	name      string
	yearBirth int32
}

func TestDbUsage(t *testing.T) {
	db, err := sql.Open("mysql", "root:password@tcp(localhost:3306)/test")
	if err != nil {
		t.Error(err)
		return
	}

	defer db.Close()

	result, err := db.Exec("insert into member(name, year_birth) values(?, ?)", "Andy's", 1990)
	if err != nil {
		t.Error(err)
		return
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		t.Error(err)
		return
	}

	fmt.Println(lastID)

	ctx := context.Background()
	SetupLogger(LogLevelDebug, os.Stdout)
	mapper := RowMapperFunc(func(row Scannable) (interface{}, error) {
		m := &member{}
		err = row.Scan(&m.id, &m.name, &m.yearBirth)
		return m, err
	})

	obj, err := QueryOne(ctx, db, mapper, "select id, name, year_birth from member where id=?", lastID)
	if err != nil {
		t.Error(err)
		return
	}

	m := obj.(*member)
	fmt.Println(m.id, m.name, m.yearBirth)

	objs, err := QueryAll(ctx, db, mapper, "select id, name, year_birth from member order by id asc")
	if err != nil {
		t.Error(err)
		return
	}

	for _, obj = range objs {
		m = obj.(*member)
		fmt.Println(m.id, m.name, m.yearBirth)
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: false})
	if err != nil {
		t.Error(err)
		return
	}

	defer tx.Rollback()
	objs, err = QueryAll(ctx, db, mapper, "select id, name, year_birth from member order by id asc")
	if err != nil {
		t.Error(err)
		return
	}

	for _, obj = range objs {
		m = obj.(*member)
		fmt.Println(m.id, m.name, m.yearBirth)
	}
}
