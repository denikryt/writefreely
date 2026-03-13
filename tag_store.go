/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"database/sql"
	"strings"

	"github.com/gosimple/slug"
)

type Tag struct {
	ID           int64
	CollectionID int64
	Name         string
	Slug         string
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

func normalizePostTags(raw *string) []string {
	if raw == nil {
		return nil
	}
	return normalizeTags(*raw)
}

func (db *datastore) GetPostTags(postID string) ([]string, error) {
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

func (db *datastore) loadPostTags(p *Post) error {
	tags, err := db.GetPostTags(p.ID)
	if err != nil {
		return err
	}
	p.Tags = tags
	return nil
}

func (db *datastore) getSingleOwnerCollectionID(tx *sql.Tx, ownerID int64) (int64, error) {
	rows, err := tx.Query("SELECT id FROM collections WHERE owner_id = ? ORDER BY id ASC LIMIT 2", ownerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	collectionIDs := []int64{}
	for rows.Next() {
		var collectionID int64
		if err = rows.Scan(&collectionID); err != nil {
			return 0, err
		}
		collectionIDs = append(collectionIDs, collectionID)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	if len(collectionIDs) == 1 {
		return collectionIDs[0], nil
	}
	return 0, nil
}

func (db *datastore) AssignPostTags(postID string, collectionID int64, tags []string) error {
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
