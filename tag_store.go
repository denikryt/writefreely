package writefreely

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/gosimple/slug"
)

type Tag struct {
	ID           int64  `db:"id" json:"id"`
	CollectionID int64  `db:"collection_id" json:"collection_id"`
	Name         string `db:"name" json:"name"`
	Slug         string `db:"slug" json:"slug"`
}

func normalizeTags(raw string) []string {
	if raw == "" {
		return nil
	}

	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		normalized := slug.Make(name)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func formatTags(tags []string) string {
	return strings.Join(tags, ", ")
}

func (db *datastore) GetPostTags(postID string) ([]string, error) {
	if err := db.ensureExplicitTagsSchema(); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT t.slug
FROM tags t
INNER JOIN post_tags pt ON pt.tag_id = t.id
WHERE pt.post_id = ?
ORDER BY pt.position ASC, t.slug ASC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err = rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (db *datastore) AssignPostTags(postID string, collectionID int64, tags []string) error {
	if err := db.ensureExplicitTagsSchema(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if err = db.assignPostTagsTx(tx, postID, collectionID, tags); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (db *datastore) assignPostTagsTx(tx *sql.Tx, postID string, collectionID int64, tags []string) error {
	if _, err := tx.Exec("DELETE FROM post_tags WHERE post_id = ?", postID); err != nil {
		return err
	}
	if collectionID <= 0 || len(tags) == 0 {
		return nil
	}

	for idx, tag := range tags {
		var tagID int64
		err := tx.QueryRow("SELECT id FROM tags WHERE collection_id = ? AND slug = ?", collectionID, tag).Scan(&tagID)
		switch {
		case err == sql.ErrNoRows:
			res, insertErr := tx.Exec("INSERT INTO tags (collection_id, name, slug) VALUES (?, ?, ?)", collectionID, tag, tag)
			if insertErr != nil {
				if !db.isDuplicateKeyErr(insertErr) {
					return insertErr
				}
				if err = tx.QueryRow("SELECT id FROM tags WHERE collection_id = ? AND slug = ?", collectionID, tag).Scan(&tagID); err != nil {
					return err
				}
			} else {
				tagID, err = res.LastInsertId()
				if err != nil {
					if err = tx.QueryRow("SELECT id FROM tags WHERE collection_id = ? AND slug = ?", collectionID, tag).Scan(&tagID); err != nil {
						return err
					}
				}
			}
		case err != nil:
			return err
		}

		if _, err = tx.Exec("INSERT INTO post_tags (post_id, tag_id, position) VALUES (?, ?, ?)", postID, tagID, idx); err != nil {
			return err
		}
	}

	return nil
}

func (db *datastore) ensureExplicitTagsSchema() error {
	db.explicitTagsSchemaOnce.Do(func() {
		db.explicitTagsSchemaErr = db.ensureExplicitTagsSchemaInner()
	})
	return db.explicitTagsSchemaErr
}

func (db *datastore) ensureExplicitTagsSchemaInner() error {
	db.explicitTagsSchemaMu.Lock()
	defer db.explicitTagsSchemaMu.Unlock()

	if db.driverName == driverSQLite {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tags (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  CONSTRAINT collection_id_slug UNIQUE (collection_id, slug)
)`); err != nil {
			return err
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS post_tags (
  post_id TEXT NOT NULL,
  tag_id INTEGER NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (post_id, tag_id)
)`); err != nil {
			return err
		}
		if !db.columnExists("post_tags", "position") {
			if _, err := db.Exec(`ALTER TABLE post_tags ADD COLUMN position INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
		}
		return nil
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tags (
  id INT AUTO_INCREMENT PRIMARY KEY,
  collection_id INT NOT NULL,
  name VARCHAR(255) COLLATE utf8_bin NOT NULL,
  slug VARCHAR(255) COLLATE utf8_bin NOT NULL,
  UNIQUE KEY collection_id_slug (collection_id, slug)
) ENGINE=InnoDB`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS post_tags (
  post_id CHAR(16) NOT NULL,
  tag_id INT NOT NULL,
  position INT NOT NULL DEFAULT 0,
  PRIMARY KEY (post_id, tag_id),
  KEY tag_id (tag_id)
) ENGINE=InnoDB`); err != nil {
		return err
	}
	if !db.columnExists("post_tags", "position") {
		if _, err := db.Exec(`ALTER TABLE post_tags ADD COLUMN position INT NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

func (db *datastore) columnExists(table, column string) bool {
	var dummy string
	var err error
	if db.driverName == driverSQLite {
		rows, qErr := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if qErr != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt sql.NullString
			if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); scanErr != nil {
				return false
			}
			if name == column {
				return true
			}
		}
		return false
	}

	err = db.QueryRow(`SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&dummy)
	return err == nil && dummy == column
}
