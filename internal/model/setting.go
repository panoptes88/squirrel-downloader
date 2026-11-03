package model

import "gorm.io/gorm"

// Setting 是运行期可调整的配置项（键值对）。
// 环境变量只在数据库里没有对应键时作为初始值，页面上的修改会覆盖它并持久化。
type Setting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"size:256" json:"value"`
}

func (Setting) TableName() string {
	return "settings"
}

func GetSetting(db *gorm.DB, key string) (string, bool) {
	var s Setting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		return "", false
	}
	return s.Value, true
}

func SetSetting(db *gorm.DB, key, value string) error {
	return db.Save(&Setting{Key: key, Value: value}).Error
}
