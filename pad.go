/*
 * Copyright © 2018-2021 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/writeas/impart"
	"github.com/writeas/web-core/log"
	"github.com/writefreely/writefreely/page"
)

type padPageData struct {
	page.StaticPage
	Post                  *RawPost
	User                  *User
	Blogs                 *[]Collection
	Silenced              bool
	Editing               bool
	EditCollection        *Collection
	AvailableCategories   []Category
	CategoriesByBlogJSON  template.JS
	SelectedCategorySlugs template.JS
}

type metaPageData struct {
	page.StaticPage
	Post                *RawPost
	User                *User
	EditCollection      *Collection
	Flashes             []string
	NeedsToken          bool
	Silenced            bool
	AvailableCategories []Category
	SelectedCategories  map[string]bool
}

func mustCategoriesJSONByBlog(categoriesByBlog map[string][]Category) template.JS {
	if categoriesByBlog == nil {
		return template.JS("{}")
	}
	b, err := json.Marshal(categoriesByBlog)
	if err != nil {
		log.Error("Unable to marshal categories by blog: %v", err)
		return template.JS("{}")
	}
	return template.JS(b)
}

func mustSelectedCategorySlugsJSON(categories []Category) template.JS {
	slugs := make([]string, 0, len(categories))
	for _, category := range categories {
		slugs = append(slugs, category.Slug)
	}
	b, err := json.Marshal(slugs)
	if err != nil {
		log.Error("Unable to marshal selected categories: %v", err)
		return template.JS("[]")
	}
	return template.JS(b)
}

func handleViewPad(app *App, w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	action := vars["action"]
	slug := vars["slug"]
	collAlias := vars["collection"]
	if app.cfg.App.SingleUser {
		// TODO: refactor all of this, especially for single-user blogs
		c, err := app.db.GetCollectionByID(1)
		if err != nil {
			return err
		}
		collAlias = c.Alias
	}
	appData := &padPageData{
		StaticPage:            pageForReq(app, r),
		Post:                  &RawPost{Font: "norm"},
		User:                  getUserSession(app, r),
		CategoriesByBlogJSON:  template.JS("{}"),
		SelectedCategorySlugs: template.JS("[]"),
	}
	var err error
	if appData.User != nil {
		appData.Blogs, err = app.db.GetPublishableCollections(appData.User, app.cfg.App.Host)
		if err != nil {
			log.Error("Unable to get user's blogs for Pad: %v", err)
		}
		appData.Silenced, err = app.db.IsUserSilenced(appData.User.ID)
		if err != nil {
			if err == ErrUserNotFound {
				return err
			}
			log.Error("Unable to get user status for Pad: %v", err)
		}
		categoriesByBlog := map[string][]Category{}
		if appData.Blogs != nil {
			for _, blog := range *appData.Blogs {
				categories, catErr := app.db.GetCategoriesByCollection(blog.ID)
				if catErr != nil {
					log.Error("Unable to get categories for %s: %v", blog.Alias, catErr)
					continue
				}
				categoriesByBlog[blog.Alias] = categories
			}
		}
		appData.CategoriesByBlogJSON = mustCategoriesJSONByBlog(categoriesByBlog)
	}

	padTmpl := app.cfg.App.Editor
	if templates[padTmpl] == nil {
		if padTmpl != "" {
			log.Info("No template '%s' found. Falling back to default 'pad' template.", padTmpl)
		}
		padTmpl = "pad"
	}

	if action == "" && slug == "" {
		// Not editing any post; simply render the Pad
		if err = templates[padTmpl].ExecuteTemplate(w, "pad", appData); err != nil {
			log.Error("Unable to execute template: %v", err)
		}

		return nil
	}

	// Retrieve post information for editing
	appData.Editing = true
	// Make sure this isn't cached, so user doesn't accidentally lose data
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Expires", "Thu, 04 Oct 1990 20:00:00 GMT")
	if slug != "" {
		// TODO: refactor all of this, especially for single-user blogs
		appData.Post = getRawCollectionPost(app, slug, collAlias)
		if appData.Post.OwnerID != appData.User.ID {
			// TODO: add ErrForbiddenEditPost message to flashes
			return impart.HTTPError{http.StatusFound, r.URL.Path[:strings.LastIndex(r.URL.Path, "/edit")]}
		}
		appData.EditCollection, err = app.db.GetCollectionForPad(collAlias)
		if err != nil {
			log.Error("Unable to GetCollectionForPad: %s", err)
			return err
		}
		appData.EditCollection.hostName = app.cfg.App.Host
		appData.AvailableCategories, _ = app.db.GetCategoriesByCollection(appData.EditCollection.ID)
		appData.SelectedCategorySlugs = mustSelectedCategorySlugsJSON(appData.Post.Categories)
	} else {
		// Editing a floating article
		appData.Post = getRawPost(app, action)
		appData.Post.Id = action
		appData.SelectedCategorySlugs = mustSelectedCategorySlugsJSON(appData.Post.Categories)
	}

	if appData.Post.Gone {
		return ErrPostUnpublished
	} else if appData.Post.Found && (appData.Post.Title != "" || appData.Post.Content != "") {
		// Got the post
	} else if appData.Post.Found {
		log.Error("Found post, but other conditions failed.")
		return ErrPostFetchError
	} else {
		return ErrPostNotFound
	}

	if err = templates[padTmpl].ExecuteTemplate(w, "pad", appData); err != nil {
		log.Error("Unable to execute template: %v", err)
	}
	return nil
}

func handleViewMeta(app *App, w http.ResponseWriter, r *http.Request) error {
	vars := mux.Vars(r)
	action := vars["action"]
	slug := vars["slug"]
	collAlias := vars["collection"]
	appData := &metaPageData{
		StaticPage:         pageForReq(app, r),
		Post:               &RawPost{Font: "norm"},
		User:               getUserSession(app, r),
		SelectedCategories: map[string]bool{},
	}
	var err error
	appData.Silenced, err = app.db.IsUserSilenced(appData.User.ID)
	if err != nil {
		log.Error("view meta: get user status: %v", err)
		return ErrInternalGeneral
	}

	if action == "" && slug == "" {
		return ErrPostNotFound
	}

	// Make sure this isn't cached, so user doesn't accidentally lose data
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Expires", "Thu, 28 Jul 1989 12:00:00 GMT")
	if slug != "" {
		appData.Post = getRawCollectionPost(app, slug, collAlias)
		if appData.Post.OwnerID != appData.User.ID {
			// TODO: add ErrForbiddenEditPost message to flashes
			return impart.HTTPError{http.StatusFound, r.URL.Path[:strings.LastIndex(r.URL.Path, "/meta")]}
		}
		if app.cfg.App.SingleUser {
			// TODO: optimize this query just like we do in GetCollectionForPad (?)
			appData.EditCollection, err = app.db.GetCollectionByID(1)
		} else {
			appData.EditCollection, err = app.db.GetCollectionForPad(collAlias)
		}
		if err != nil {
			return err
		}
		appData.EditCollection.hostName = app.cfg.App.Host
		appData.AvailableCategories, _ = app.db.GetCategoriesByCollection(appData.EditCollection.ID)
		appData.SelectedCategories = selectedCategoryMap(appData.Post.Categories)
	} else {
		// Editing a floating article
		appData.Post = getRawPost(app, action)
		appData.Post.Id = action
		if app.cfg.App.SingleUser {
			appData.EditCollection, err = app.db.GetCollectionByID(1)
			if err != nil {
				return err
			}
			appData.EditCollection.hostName = app.cfg.App.Host
			appData.AvailableCategories, _ = app.db.GetCategoriesByCollection(appData.EditCollection.ID)
			appData.SelectedCategories = selectedCategoryMap(appData.Post.Categories)
		}
	}
	appData.NeedsToken = appData.User == nil || appData.User.ID != appData.Post.OwnerID

	if appData.Post.Gone {
		return ErrPostUnpublished
	} else if appData.Post.Found && appData.Post.Content != "" {
		// Got the post
	} else if appData.Post.Found {
		return ErrPostFetchError
	} else {
		return ErrPostNotFound
	}
	appData.Flashes, _ = getSessionFlashes(app, w, r, nil)

	if err = templates["edit-meta"].ExecuteTemplate(w, "edit-meta", appData); err != nil {
		log.Error("Unable to execute template: %v", err)
	}
	return nil
}
