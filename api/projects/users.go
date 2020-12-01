package projects

import (
	"database/sql"
	"github.com/ansible-semaphore/semaphore/db"
	"github.com/ansible-semaphore/semaphore/models"
	"net/http"
	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/ansible-semaphore/semaphore/util"
	"github.com/gorilla/context"
	"github.com/masterminds/squirrel"
)

// UserMiddleware ensures a user exists and loads it to the context
func UserMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := context.Get(r, "project").(models.Project)
		userID, err := util.GetIntParam("user_id", w, r)
		if err != nil {
			return
		}

		var user models.User
		if err := context.Get(r, "store").(db.Store).Sql().SelectOne(&user, "select u.* from project__user as pu join `user` as u on pu.user_id=u.id where pu.user_id=? and pu.project_id=?", userID, project.ID); err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			panic(err)
		}

		context.Set(r, "projectUser", user)
		next.ServeHTTP(w, r)
	})
}

// GetUsers returns all users in a project
func GetUsers(w http.ResponseWriter, r *http.Request) {
	if user := context.Get(r, "projectUser"); user != nil {
		util.WriteJSON(w, http.StatusOK, user.(models.User))
		return
	}

	project := context.Get(r, "project").(models.Project)
	var users []models.User

	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	if order != asc && order != desc {
		order = asc
	}

	q := squirrel.Select("u.*").Column("pu.admin").
		From("project__user as pu").
		LeftJoin("user as u on pu.user_id=u.id").
		Where("pu.project_id=?", project.ID)

	switch sort {
	case "name", "username", "email":
		q = q.OrderBy("u." + sort + " " + order)
	case "admin":
		q = q.OrderBy("pu." + sort + " " + order)
	default:
		q = q.OrderBy("u.name " + order)
	}

	query, args, _ := q.ToSql()

	if _, err := context.Get(r, "store").(db.Store).Sql().Select(&users, query, args...); err != nil {
		panic(err)
	}

	util.WriteJSON(w, http.StatusOK, users)
}

// AddUser adds a user to a projects team in the database
func AddUser(w http.ResponseWriter, r *http.Request) {
	project := context.Get(r, "project").(models.Project)
	var user struct {
		UserID int  `json:"user_id" binding:"required"`
		Admin  bool `json:"admin"`
	}

	err := util.Bind(w, r, &user)

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	_, err = context.Get(r, "store").(db.Store).CreateProjectUser(models.ProjectUser{ProjectID: project.ID, UserID: user.UserID, Admin: user.Admin})

	if err != nil {
		w.WriteHeader(http.StatusConflict)
		return
	}

	objType := "user"
	desc := "User ID " + strconv.Itoa(user.UserID) + " added to team"

	_, err = context.Get(r, "store").(db.Store).CreateEvent(models.Event{
		ProjectID:   &project.ID,
		ObjectType:  &objType,
		ObjectID:    &user.UserID,
		Description: &desc,
	})

	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveUser removes a user from a project team
func RemoveUser(w http.ResponseWriter, r *http.Request) {
	project := context.Get(r, "project").(models.Project)
	user := context.Get(r, "projectUser").(models.User)

	err := context.Get(r, "store").(db.Store).DeleteProjectUser(project.ID, user.ID)

	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	objType := "user"
	desc := "User ID " + strconv.Itoa(user.ID) + " removed from team"

	_, err = context.Get(r, "store").(db.Store).CreateEvent(models.Event{
		ProjectID:   &project.ID,
		ObjectType:  &objType,
		ObjectID:    &user.ID,
		Description: &desc,
	})

	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// MakeUserAdmin writes the admin flag to the users account
func MakeUserAdmin(w http.ResponseWriter, r *http.Request) {
	project := context.Get(r, "project").(models.Project)
	user := context.Get(r, "projectUser").(models.User)
	admin := 1

	if r.Method == "DELETE" {
		// strip admin
		admin = 0
	}

	if _, err := context.Get(r, "store").(db.Store).Sql().Exec("update project__user set `admin`=? where user_id=? and project_id=?", admin, user.ID, project.ID); err != nil {
		panic(err)
	}

	w.WriteHeader(http.StatusNoContent)
}
