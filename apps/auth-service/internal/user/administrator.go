package user

import "gorm.io/gorm"

// Administrator is the write interface for user data access.
type Administrator interface {
	Insert(Model) (Model, error)
	Update(Model) (Model, error)
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }

func (s *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}

func (s *dbAdministrator) Update(m Model) (Model, error) {
	e := m.ToEntity()
	if err := s.db.Save(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
