package user

import (
	"errors"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

type Provider interface{ GetBySub(sub string) (Model, error) }

type Writer interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
}

type dbStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *dbStore { return &dbStore{db: db} }

func (s *dbStore) GetBySub(sub string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("google_sub = ?", sub).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}

func (s *dbStore) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (s *dbStore) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Save(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
