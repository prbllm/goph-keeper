// Package model определяет структуры полезной нагрузки для различных типов секретов.
// Предоставляет функции сериализации данных в JSON формат.
package model

import "encoding/json"

// CredentialPayload представляет учётные данные (логин и пароль).
// Используется для хранения пар логин/пароль в зашифрованном виде.
type CredentialPayload struct {
	// Login — имя пользователя или электронная почта
	Login string `json:"login"`
	// Password — пароль или секретный ключ
	Password string `json:"password"`
}

// CardPayload представляет данные платёжной карты.
// Содержит номер карты, имя держателя, срок действия и CVV код.
type CardPayload struct {
	// Number — номер платёжной карты
	Number string `json:"number"`
	// Holder — имя владельца карты
	Holder string `json:"holder"`
	// Exp — срок действия карты (ММ/ГГ)
	Exp string `json:"exp"`
	// CVV — проверочный код карты (3-4 цифры)
	CVV string `json:"cvv"`
}

// EncodePayload сериализует произвольную структуру в JSON формат.
// Принимает интерфейсное значение любой структуры.
// Возвращает байтовый срез с JSON данными или ошибку сериализации.
func EncodePayload(v any) ([]byte, error) {
	return json.Marshal(v)
}
