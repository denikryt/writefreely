package writefreely

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/writeas/impart"
	corecategory "github.com/writeas/web-core/category"
	"github.com/writeas/web-core/log"
)

type Category struct {
	corecategory.Category
	CollectionID int64  `json:"collection_id"`
	Description  string `json:"description,omitempty"`
}

type SubmittedCategory struct {
	Title       string `schema:"title" json:"title"`
	Slug        string `schema:"slug" json:"slug"`
	Description string `schema:"description" json:"description"`
}

type CategoryCollectionPage struct {
	CollectionPage
	Category *Category
}

func categoryCustomizeRedirect(alias string) impart.HTTPError {
	return impart.HTTPError{Status: http.StatusFound, Message: "/me/c/" + alias}
}

func decodeSubmittedCategory(app *App, r *http.Request) (*SubmittedCategory, error) {
	var submitted SubmittedCategory
	if IsJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
			return nil, ErrBadJSON
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return nil, ErrBadFormData
		}
		if err := app.formDecoder.Decode(&submitted, r.PostForm); err != nil {
			return nil, ErrBadFormData
		}
	}
	if strings.TrimSpace(submitted.Title) == "" {
		return nil, impart.HTTPError{Status: http.StatusBadRequest, Message: "Category title is required."}
	}
	return &submitted, nil
}

func writeCategoryActionResult(app *App, w http.ResponseWriter, r *http.Request, reqJSON bool, alias, flash string, category *Category, status int) error {
	if !reqJSON {
		addSessionFlash(app, w, r, flash, nil)
		return categoryCustomizeRedirect(alias)
	}
	return impart.WriteSuccess(w, category, status)
}

func writeCategoryActionError(app *App, w http.ResponseWriter, r *http.Request, reqJSON bool, alias string, err error) error {
	if !reqJSON {
		if httpErr, ok := err.(impart.HTTPError); ok {
			addSessionFlash(app, w, r, httpErr.Message, nil)
			return categoryCustomizeRedirect(alias)
		}
	}
	return err
}

func loadOwnedCategoryCollection(app *App, r *http.Request) (*Collection, map[string]string, error) {
	vars := mux.Vars(r)
	coll, err := app.db.GetCollection(vars["alias"])
	if err != nil {
		return nil, nil, err
	}
	if _, err := collectionOwnerIDForRequest(app, r, coll); err != nil {
		return nil, nil, err
	}
	return coll, vars, nil
}

func newCategoryFromSubmitted(collID int64, submitted *SubmittedCategory) *Category {
	base := corecategory.NewCategoryFromPartial(&corecategory.Category{
		Hashtag:    corecategory.HashtagFromTitle(submitted.Title),
		Slug:       submitted.Slug,
		Title:      submitted.Title,
		IsCategory: true,
	})
	return &Category{
		Category:     *base,
		CollectionID: collID,
		Description:  submitted.Description,
	}
}

func hydrateCategory(id, collID, postCount int64, slug, title, description string) Category {
	return Category{
		Category: corecategory.Category{
			ID:         id,
			Hashtag:    corecategory.HashtagFromTitle(title),
			Slug:       slug,
			Title:      title,
			PostCount:  postCount,
			IsCategory: true,
		},
		CollectionID: collID,
		Description:  description,
	}
}

func normalizeCategorySlugs(slugs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" {
			continue
		}
		slug = corecategory.NewCategoryFromPartial(&corecategory.Category{Slug: slug, Title: slug}).Slug
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	return out
}

func selectedCategoryMap(categories []Category) map[string]bool {
	selected := make(map[string]bool, len(categories))
	for _, category := range categories {
		selected[category.Slug] = true
	}
	return selected
}

func collectionOwnerIDForRequest(app *App, r *http.Request, coll *Collection) (int64, error) {
	userID, err := apiCheckCollectionPermissions(app, r, coll)
	if err != nil {
		return -1, err
	}
	if userID == -1 {
		if user := getUserSession(app, r); user != nil {
			userID = user.ID
		}
	}
	if userID != coll.OwnerID {
		return -1, ErrForbiddenCollection
	}
	return userID, nil
}

func (ccp CategoryCollectionPage) PrevPageURL(prefix string, n int, tl bool) string {
	u := fmt.Sprintf("/category/%s", ccp.Category.Slug)
	if n > 2 {
		u += fmt.Sprintf("/page/%d", n-1)
	}
	if tl {
		return u
	}
	return "/" + prefix + ccp.Alias + u
}

func (ccp CategoryCollectionPage) NextPageURL(prefix string, n int, tl bool) string {
	if tl {
		return fmt.Sprintf("/category/%s/page/%d", ccp.Category.Slug, n+1)
	}
	return fmt.Sprintf("/%s%s/category/%s/page/%d", prefix, ccp.Alias, ccp.Category.Slug, n+1)
}

func fetchCollectionCategories(app *App, w http.ResponseWriter, r *http.Request) error {
	c, _, err := loadOwnedCategoryCollection(app, r)
	if err != nil {
		return err
	}

	categories, err := app.db.GetCategoriesByCollection(c.ID)
	if err != nil {
		return err
	}
	return impart.WriteSuccess(w, categories, http.StatusOK)
}

func fetchCollectionCategory(app *App, w http.ResponseWriter, r *http.Request) error {
	c, vars, err := loadOwnedCategoryCollection(app, r)
	if err != nil {
		return err
	}

	category, err := app.db.GetCategoryBySlug(c.ID, vars["slug"])
	if err != nil {
		return err
	}
	return impart.WriteSuccess(w, category, http.StatusOK)
}

func createCollectionCategory(app *App, w http.ResponseWriter, r *http.Request) error {
	coll, vars, err := loadOwnedCategoryCollection(app, r)
	if err != nil {
		return err
	}
	reqJSON := IsJSON(r)

	submitted, err := decodeSubmittedCategory(app, r)
	if err != nil {
		return err
	}

	category, err := app.db.CreateCategory(coll.ID, submitted)
	if err != nil {
		return writeCategoryActionError(app, w, r, reqJSON, vars["alias"], err)
	}
	return writeCategoryActionResult(app, w, r, reqJSON, vars["alias"], "Category created!", category, http.StatusCreated)
}

func updateCollectionCategory(app *App, w http.ResponseWriter, r *http.Request) error {
	coll, vars, err := loadOwnedCategoryCollection(app, r)
	if err != nil {
		return err
	}
	reqJSON := IsJSON(r)

	submitted, err := decodeSubmittedCategory(app, r)
	if err != nil {
		return err
	}

	category, err := app.db.UpdateCategory(coll.ID, vars["slug"], submitted)
	if err != nil {
		return writeCategoryActionError(app, w, r, reqJSON, vars["alias"], err)
	}
	return writeCategoryActionResult(app, w, r, reqJSON, vars["alias"], "Category updated!", category, http.StatusOK)
}

func assignPostCategories(app *App, w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	coll, err := app.db.GetCollection(vars["alias"])
	if err != nil {
		return err
	}
	_, err = collectionOwnerIDForRequest(app, r, coll)
	if err != nil {
		return err
	}

	var payload struct {
		Categories []string `json:"categories" schema:"categories"`
	}
	if IsJSON(r) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			return ErrBadJSON
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return ErrBadFormData
		}
		if err := app.formDecoder.Decode(&payload, r.PostForm); err != nil {
			return ErrBadFormData
		}
	}

	if err := app.db.AssignPostCategories(vars["post"], coll.ID, normalizeCategorySlugs(payload.Categories)); err != nil {
		return err
	}
	return impart.WriteSuccess(w, map[string]bool{"ok": true}, http.StatusOK)
}

func deleteCollectionCategory(app *App, w http.ResponseWriter, r *http.Request) error {
	coll, vars, err := loadOwnedCategoryCollection(app, r)
	if err != nil {
		return err
	}
	reqJSON := IsJSON(r)

	if err := app.db.DeleteCategory(coll.ID, vars["slug"]); err != nil {
		return writeCategoryActionError(app, w, r, reqJSON, vars["alias"], err)
	}

	if !reqJSON {
		addSessionFlash(app, w, r, "Category deleted!", nil)
		return categoryCustomizeRedirect(vars["alias"])
	}
	return impart.HTTPError{Status: http.StatusNoContent}
}

func handleViewCategory(app *App, w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	categorySlug := vars["slug"]

	cr := &collectionReq{}
	if err := processCollectionRequest(cr, vars, w, r); err != nil {
		return err
	}

	u, err := checkUserForCollection(app, cr, r, false)
	if err != nil {
		return err
	}

	page := getCollectionPage(vars)
	c, err := processCollectionPermissions(app, cr, u, w, r)
	if c == nil || err != nil {
		return err
	}

	coll, err := newDisplayCollection(c, cr, page)
	if err != nil {
		return err
	}

	category, err := app.db.GetCategoryBySlug(c.ID, categorySlug)
	if err != nil {
		return err
	}

	coll.NavSuffix = fmt.Sprintf("/category/%s", category.Slug)
	totalPosts, err := app.db.GetCategoryPostCount(c.ID, category.Slug, cr.isCollOwner)
	if err != nil {
		return err
	}

	pagePosts := coll.Format.PostsPerPage()
	coll.TotalPages = int(math.Ceil(float64(totalPosts) / float64(pagePosts)))
	if coll.TotalPages > 0 && page > coll.TotalPages {
		redirURL := fmt.Sprintf("/category/%s/page/%d", category.Slug, coll.TotalPages)
		if !app.cfg.App.SingleUser {
			redirURL = fmt.Sprintf("/%s%s%s", cr.prefix, coll.Alias, redirURL)
		}
		return impart.HTTPError{http.StatusFound, redirURL}
	}

	coll.Posts, err = app.db.GetPostsByCategory(app.cfg, c, category.Slug, page, cr.isCollOwner)
	if err != nil {
		return err
	}
	if coll.Posts != nil && len(*coll.Posts) == 0 && page > 1 {
		return ErrCollectionPageNotFound
	}

	displayPage := CategoryCollectionPage{
		CollectionPage: CollectionPage{
			DisplayCollection: coll,
			StaticPage:        pageForReq(app, r),
			IsCustomDomain:    cr.isCustomDomain,
			Username:          "",
		},
		Category: category,
	}
	var owner *User
	if u != nil {
		displayPage.Username = u.Username
		displayPage.IsOwner = u.ID == coll.OwnerID
		if displayPage.IsOwner {
			owner = u
			displayPage.CanPin = true

			pubColls, err := app.db.GetPublishableCollections(owner, app.cfg.App.Host)
			if err != nil {
				log.Error("unable to fetch collections: %v", err)
			}
			displayPage.Collections = pubColls
		}
	}
	isOwner := owner != nil
	if !isOwner {
		owner, err = app.db.GetUserByID(coll.OwnerID)
		if err != nil {
			log.Error("Error getting user for collection: %v", err)
		}
		if owner != nil && owner.IsSilenced() {
			return ErrCollectionNotFound
		}
	}
	displayPage.Silenced = owner != nil && owner.IsSilenced()
	displayPage.Owner = owner
	coll.Owner = displayPage.Owner
	displayPage.PinnedPosts, _ = app.db.GetPinnedPosts(coll.CollectionObj, isOwner)
	displayPage.Monetization = app.db.GetCollectionAttribute(coll.ID, "monetization_pointer")

	if err := templates["category"].ExecuteTemplate(w, "category", displayPage); err != nil {
		log.Error("Unable to render collection category page: %v", err)
	}

	return nil
}
