package migrations

func supportCategories(db *datastore) error {
	t, err := db.Begin()
	if err != nil {
		t.Rollback()
		return err
	}

	_, err = t.Exec(`CREATE TABLE IF NOT EXISTS categories (
	id            ` + db.typeIntPrimaryKey() + `,
	collection_id ` + db.typeInt() + ` NOT NULL,
	slug          ` + db.typeVarChar(100) + ` NOT NULL,
	title         ` + db.typeVarChar(255) + db.collateMultiByte() + ` NOT NULL,
	description   ` + db.typeText() + db.collateMultiByte() + ` NOT NULL,
	UNIQUE(collection_id, slug)
)` + db.engine())
	if err != nil {
		t.Rollback()
		return err
	}

	_, err = t.Exec(`CREATE TABLE IF NOT EXISTS post_categories (
	post_id     ` + db.typeChar(16) + ` NOT NULL,
	category_id ` + db.typeInt() + ` NOT NULL,
	PRIMARY KEY (post_id, category_id)
)` + db.engine())
	if err != nil {
		t.Rollback()
		return err
	}

	if err = t.Commit(); err != nil {
		t.Rollback()
		return err
	}
	return nil
}
