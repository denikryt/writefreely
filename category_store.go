package writefreely

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/writeas/impart"
	"github.com/writeas/web-core/log"
	"github.com/writefreely/writefreely/config"
)

type categoryRowScanner interface {
	Scan(dest ...interface{}) error
}

type categoryRowQuerier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func scanCategory(scanner categoryRowScanner, withPostCount bool, slug string) (*Category, error) {
	var id, collID int64
	var title, description string
	if withPostCount {
		var postCount int64
		if err := scanner.Scan(&id, &collID, &title, &description, &postCount); err != nil {
			return nil, err
		}
		category := hydrateCategory(id, collID, postCount, slug, title, description)
		return &category, nil
	}
	if err := scanner.Scan(&id, &collID, &slug, &title, &description); err != nil {
		return nil, err
	}
	category := hydrateCategory(id, collID, 0, slug, title, description)
	return &category, nil
}

func scanCategoryList(rows *sql.Rows, withPostCount bool) ([]Category, error) {
	categories := []Category{}
	for rows.Next() {
		category, err := scanCategory(rows, withPostCount, "")
		if err != nil {
			return nil, err
		}
		categories = append(categories, *category)
	}
	return categories, rows.Err()
}

func findCategoryID(q categoryRowQuerier, collectionID int64, slug string) (int64, error) {
	var categoryID int64
	err := q.QueryRow("SELECT id FROM categories WHERE collection_id = ? AND slug = ?", collectionID, slug).Scan(&categoryID)
	switch {
	case err == sql.ErrNoRows:
		return 0, ErrCollectionPageNotFound
	case err != nil:
		return 0, err
	default:
		return categoryID, nil
	}
}

func (db *datastore) CreateCategory(collectionID int64, submitted *SubmittedCategory) (*Category, error) {
	category := newCategoryFromSubmitted(collectionID, submitted)
	res, err := db.Exec("INSERT INTO categories (collection_id, slug, title, description) VALUES (?, ?, ?, ?)", collectionID, category.Slug, category.Title, category.Description)
	if err != nil {
		if db.isDuplicateKeyErr(err) {
			return nil, impart.HTTPError{Status: http.StatusConflict, Message: "Category slug already exists on this blog."}
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	category.ID = id
	category.PostCount = 0
	return category, nil
}

func (db *datastore) UpdateCategory(collectionID int64, currentSlug string, submitted *SubmittedCategory) (*Category, error) {
	if strings.TrimSpace(submitted.Title) == "" {
		return nil, impart.HTTPError{Status: http.StatusBadRequest, Message: "Category title is required."}
	}

	existingID, err := findCategoryID(db, collectionID, currentSlug)
	if err != nil {
		return nil, err
	}

	category := newCategoryFromSubmitted(collectionID, submitted)
	res, err := db.Exec("UPDATE categories SET slug = ?, title = ?, description = ? WHERE id = ?", category.Slug, category.Title, category.Description, existingID)
	if err != nil {
		if db.isDuplicateKeyErr(err) {
			return nil, impart.HTTPError{Status: http.StatusConflict, Message: "Category slug already exists on this blog."}
		}
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, ErrCollectionPageNotFound
	}

	updated, err := db.GetCategoryBySlug(collectionID, category.Slug)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (db *datastore) DeleteCategory(collectionID int64, slug string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	categoryID, err := findCategoryID(tx, collectionID, slug)
	if err != nil {
		return err
	}

	if _, err = tx.Exec("DELETE FROM post_categories WHERE category_id = ?", categoryID); err != nil {
		return err
	}

	res, err := tx.Exec("DELETE FROM categories WHERE id = ?", categoryID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrCollectionPageNotFound
	}

	return tx.Commit()
}

func (db *datastore) GetCategoriesByCollection(collectionID int64) ([]Category, error) {
	rows, err := db.Query(`SELECT c.id, c.collection_id, c.slug, c.title, c.description, COUNT(pc.post_id) AS post_count
		FROM categories c
		LEFT JOIN post_categories pc ON pc.category_id = c.id
		WHERE c.collection_id = ?
		GROUP BY c.id, c.collection_id, c.slug, c.title, c.description
		ORDER BY c.title ASC`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCategoryList(rows, true)
}

func (db *datastore) GetCategoryBySlug(collectionID int64, slug string) (*Category, error) {
	category, err := scanCategory(db.QueryRow(`SELECT c.id, c.collection_id, c.title, c.description, COUNT(pc.post_id) AS post_count
		FROM categories c
		LEFT JOIN post_categories pc ON pc.category_id = c.id
		WHERE c.collection_id = ? AND c.slug = ?
		GROUP BY c.id, c.collection_id, c.title, c.description`, collectionID, slug), true, slug)
	switch {
	case err == sql.ErrNoRows:
		return nil, ErrCollectionPageNotFound
	case err != nil:
		return nil, err
	}
	return category, nil
}

func (db *datastore) GetPostCategories(postID string) ([]Category, error) {
	rows, err := db.Query(`SELECT c.id, c.collection_id, c.slug, c.title, c.description
		FROM categories c
		INNER JOIN post_categories pc ON pc.category_id = c.id
		WHERE pc.post_id = ?
		ORDER BY c.title ASC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCategoryList(rows, false)
}

func (db *datastore) attachPostCategories(posts ...*Post) {
	for _, post := range posts {
		if post == nil || post.ID == "" {
			continue
		}
		categories, err := db.GetPostCategories(post.ID)
		if err != nil {
			log.Error("Unable to load post categories for %s: %v", post.ID, err)
			continue
		}
		post.Categories = categories
	}
}

func (db *datastore) GetCategoryPostCount(collectionID int64, slug string, includeFuture bool) (int64, error) {
	timeCondition := ""
	if !includeFuture {
		timeCondition = "AND p.created <= " + db.now()
	}
	var count int64
	err := db.QueryRow(`SELECT COUNT(*)
		FROM posts p
		INNER JOIN post_categories pc ON pc.post_id = p.id
		INNER JOIN categories c ON c.id = pc.category_id
		WHERE p.collection_id = ? AND c.slug = ? `+timeCondition, collectionID, slug).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return count, nil
}

func (db *datastore) GetPostsByCategory(cfg *config.Config, c *Collection, slug string, page int, includeFuture bool) (*[]PublicPost, error) {
	collID := c.ID
	postColsQualified := "p.id, p.slug, p.text_appearance, p.language, p.rtl, p.privacy, p.owner_id, p.collection_id, p.pinned_position, p.created, p.updated, p.view_count, p.title, p.content"

	cf := c.NewFormat()
	order := "DESC"
	if cf.Ascending() {
		order = "ASC"
	}

	pagePosts := cf.PostsPerPage()
	start := page*pagePosts - pagePosts
	if page == 0 {
		start = 0
		pagePosts = 1000
	}
	limitStr := ""
	if page > 0 {
		limitStr = fmt.Sprintf(" LIMIT %d, %d", start, pagePosts)
	}
	timeCondition := ""
	if !includeFuture {
		timeCondition = "AND p.created <= " + db.now()
	}

	rows, err := db.Query(`SELECT `+postColsQualified+`
		FROM posts p
		INNER JOIN post_categories pc ON pc.post_id = p.id
		INNER JOIN categories c ON c.id = pc.category_id
		WHERE p.collection_id = ? AND c.slug = ? `+timeCondition+` ORDER BY p.created `+order+limitStr, collID, slug)
	if err != nil {
		log.Error("Failed selecting category posts: %v", err)
		return nil, impart.HTTPError{http.StatusInternalServerError, "Couldn't retrieve category posts."}
	}
	defer rows.Close()

	posts := []PublicPost{}
	for rows.Next() {
		p := &Post{}
		if err = rows.Scan(&p.ID, &p.Slug, &p.Font, &p.Language, &p.RTL, &p.Privacy, &p.OwnerID, &p.CollectionID, &p.PinnedPosition, &p.Created, &p.Updated, &p.ViewCount, &p.Title, &p.Content); err != nil {
			log.Error("Failed scanning row: %v", err)
			break
		}
		db.attachPostCategories(p)
		p.extractData()
		p.augmentContent(c)
		p.formatContent(cfg, c, includeFuture, false)
		posts = append(posts, p.processPost())
	}
	if err = rows.Err(); err != nil {
		log.Error("Error after Next() on rows: %v", err)
	}
	return &posts, nil
}

func (db *datastore) AssignPostCategories(postID string, collID int64, categorySlugs []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := db.assignPostCategoriesTx(tx, postID, collID, categorySlugs); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *datastore) assignPostCategoriesTx(tx *sql.Tx, postID string, collID int64, categorySlugs []string) error {
	if _, err := tx.Exec("DELETE FROM post_categories WHERE post_id = ?", postID); err != nil {
		return err
	}

	slugs := normalizeCategorySlugs(categorySlugs)
	if len(slugs) == 0 {
		return nil
	}

	query := fmt.Sprintf("SELECT id, slug FROM categories WHERE collection_id = ? AND slug IN (%s)", sqlPlaceholders(len(slugs)))
	args := make([]interface{}, 0, len(slugs)+1)
	args = append(args, collID)
	for _, slug := range slugs {
		args = append(args, slug)
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type categoryRow struct {
		id   int64
		slug string
	}
	found := []categoryRow{}
	for rows.Next() {
		var row categoryRow
		if err := rows.Scan(&row.id, &row.slug); err != nil {
			return err
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(found) != len(slugs) {
		return impart.HTTPError{Status: http.StatusBadRequest, Message: "One or more categories do not exist on this blog."}
	}

	for _, row := range found {
		if _, err := tx.Exec("INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)", postID, row.id); err != nil {
			return err
		}
	}
	return nil
}
