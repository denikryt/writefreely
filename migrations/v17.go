package migrations

func supportExplicitTags(db *datastore) error {
	t, err := db.Begin()
	if err != nil {
		t.Rollback()
		return err
	}

	_, err = t.Exec(`CREATE TABLE tags (
	id            ` + db.typeIntPrimaryKey() + `,
	collection_id ` + db.typeInt() + ` NOT NULL,
	name          ` + db.typeVarChar(255) + db.collateMultiByte() + ` NOT NULL,
	slug          ` + db.typeVarChar(255) + db.collateMultiByte() + ` NOT NULL,
	UNIQUE (collection_id, slug)
)` + db.engine())
	if err != nil {
		t.Rollback()
		return err
	}

	_, err = t.Exec(`CREATE TABLE post_tags (
	post_id ` + db.typeChar(16) + ` NOT NULL,
	tag_id  ` + db.typeInt() + ` NOT NULL,
	position ` + db.typeInt() + ` NOT NULL,
	PRIMARY KEY (post_id, tag_id)
)` + db.engine())
	if err != nil {
		t.Rollback()
		return err
	}

	if db.driverName != driverSQLite {
		_, err = t.Exec(`CREATE INDEX tag_id ON post_tags (tag_id)`)
		if err != nil {
			t.Rollback()
			return err
		}
	}

	return t.Commit()
}
