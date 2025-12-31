package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Password  string    `json:"-"` // tidak dikirim ke frontend
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// GET /api/users
func GetUsers(c *gin.Context) {
	rows, err := intconfig.DB.Query(`
		SELECT id, name, username, email, phone, role, status, created_at
		FROM users
		ORDER BY created_at ASC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data users: " + err.Error()})
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID,
			&u.Name,
			&u.Username,
			&u.Email,
			&u.Phone,
			&u.Role,
			&u.Status,
			&u.CreatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal parsing data users: " + err.Error()})
			return
		}
		users = append(users, u)
	}

	c.JSON(http.StatusOK, users)
}

// GET /api/users/:id
func GetUserByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var u User
	err = intconfig.DB.QueryRow(`
		SELECT id, name, username, email, phone, role, status, created_at
		FROM users WHERE id = ?
	`, id).Scan(
		&u.ID,
		&u.Name,
		&u.Username,
		&u.Email,
		&u.Phone,
		&u.Role,
		&u.Status,
		&u.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, u)
}

// POST /api/users
func CreateUser(c *gin.Context) {
	var input struct {
		Name     string `json:"name" binding:"required"`
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required"`
		Phone    string `json:"phone"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
		Status   string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "input tidak valid: " + err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal meng-hash password: " + err.Error()})
		return
	}

	if input.Role == "" {
		input.Role = "customer"
	}
	if input.Status == "" {
		input.Status = "active"
	}

	res, err := intconfig.DB.Exec(`
		INSERT INTO users (name, username, email, phone, password_hash, role, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW())
	`,
		input.Name,
		input.Username,
		input.Email,
		input.Phone,
		string(hash),
		input.Role,
		input.Status,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menyimpan user: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{
		"message": "user berhasil dibuat",
		"id":      id,
	})
}

// PUT /api/users/:id
func UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Status   string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "input tidak valid: " + err.Error()})
		return
	}

	var existing User
	err = intconfig.DB.QueryRow(`
		SELECT id, name, username, email, phone, role, status, created_at
		FROM users WHERE id = ?
	`, id).Scan(
		&existing.ID,
		&existing.Name,
		&existing.Username,
		&existing.Email,
		&existing.Phone,
		&existing.Role,
		&existing.Status,
		&existing.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user tidak ditemukan"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil user: " + err.Error()})
		return
	}

	name := existing.Name
	if input.Name != "" {
		name = input.Name
	}

	username := existing.Username
	if input.Username != "" {
		username = input.Username
	}

	email := existing.Email
	if input.Email != "" {
		email = input.Email
	}

	phone := existing.Phone
	if input.Phone != "" {
		phone = input.Phone
	}

	role := existing.Role
	if input.Role != "" {
		role = input.Role
	}
	if role == "" {
		role = "customer"
	}

	status := existing.Status
	if input.Status != "" {
		status = input.Status
	}
	if status == "" {
		status = "active"
	}

	if _, err = intconfig.DB.Exec(`
		UPDATE users
		SET name = ?, username = ?, email = ?, phone = ?, role = ?, status = ?
		WHERE id = ?
	`,
		name,
		username,
		email,
		phone,
		role,
		status,
		id,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal update user: " + err.Error()})
		return
	}

	if input.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal meng-hash password: " + err.Error()})
			return
		}

		if _, err := intconfig.DB.Exec(`
			UPDATE users SET password_hash = ? WHERE id = ?
		`, string(hash), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal update password: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "user berhasil diupdate"})
}

// DELETE /api/users/:id
func DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	if _, err := intconfig.DB.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus user: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user berhasil dihapus"})
}
