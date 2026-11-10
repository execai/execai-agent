package crypto

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	id, err := Generate()
	if err != nil {
		t.Fatalf("генерация: %v", err)
	}
	want := []byte("память агента: юзер предпочитает Go, прод на R5, ключи не синкаем")

	blob, err := Encrypt(want, id.Public())
	if err != nil {
		t.Fatalf("шифрование: %v", err)
	}
	// Шифротекст не должен содержать открытый текст — проверка от глупых ошибок
	// вроде «забыли вызвать Close» или подмены на no-op.
	if strings.Contains(string(blob), "память агента") {
		t.Fatal("открытый текст виден в шифротексте")
	}

	got, err := Decrypt(blob, id)
	if err != nil {
		t.Fatalf("расшифровка: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("данные не совпали:\n got %q\nwant %q", got, want)
	}
}

// Ради этого выбрана модель получателей: пригласить второго участника значит
// перешифровать запись на двоих, а не отдать ему свой ключ.
func TestEncrypt_MultipleRecipients(t *testing.T) {
	alice, _ := Generate()
	bob, _ := Generate()

	blob, err := Encrypt([]byte("общая память проекта"), alice.Public(), bob.Public())
	if err != nil {
		t.Fatalf("шифрование на двоих: %v", err)
	}

	for name, id := range map[string]*Identity{"alice": alice, "bob": bob} {
		got, err := Decrypt(blob, id)
		if err != nil {
			t.Errorf("%s не смог расшифровать: %v", name, err)
			continue
		}
		if string(got) != "общая память проекта" {
			t.Errorf("%s получил не то: %q", name, got)
		}
	}
}

// Чужой ключ не должен открывать блоб — иначе всё построение бессмысленно.
func TestDecrypt_ForeignKeyRejected(t *testing.T) {
	mine, _ := Generate()
	stranger, _ := Generate()

	blob, err := Encrypt([]byte("секрет"), mine.Public())
	if err != nil {
		t.Fatalf("шифрование: %v", err)
	}
	if _, err := Decrypt(blob, stranger); err == nil {
		t.Fatal("чужой ключ расшифровал блоб")
	} else if !errors.Is(err, ErrNotForYou) {
		t.Errorf("невнятная ошибка для чужого ключа: %v", err)
	}
}

// Порча шифротекста обязана ловиться, а не отдавать мусор: у age есть MAC.
func TestDecrypt_TamperedBlobRejected(t *testing.T) {
	id, _ := Generate()
	blob, _ := Encrypt([]byte("данные, которые нельзя тихо подменить"), id.Public())

	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-3] ^= 0xFF // портим байт ближе к концу — в MAC

	if _, err := Decrypt(tampered, id); err == nil {
		t.Fatal("испорченный блоб расшифровался без ошибки")
	}
}

// Пустой список получателей — блоб, который никто не прочитает. Это ошибка
// вызывающего, и поймать её можно только здесь.
func TestEncrypt_NoRecipientsRejected(t *testing.T) {
	if _, err := Encrypt([]byte("x")); err == nil {
		t.Error("шифрование без получателей должно возвращать ошибку")
	}
}

func TestSaveLoadIdentity(t *testing.T) {
	dir := t.TempDir()

	if _, err := LoadIdentity(dir); !errors.Is(err, ErrNoKey) {
		t.Errorf("на пустом каталоге ожидался ErrNoKey, получено: %v", err)
	}

	id, _ := Generate()
	if err := SaveIdentity(dir, id); err != nil {
		t.Fatalf("сохранение: %v", err)
	}

	// Ключ — секрет: файл должен быть доступен только владельцу.
	info, err := os.Stat(filepath.Join(dir, KeyFileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("режим файла ключа %o, ожидался 600", perm)
	}

	loaded, err := LoadIdentity(dir)
	if err != nil {
		t.Fatalf("загрузка: %v", err)
	}
	if loaded.String() != id.String() {
		t.Error("загруженный ключ не совпал с сохранённым")
	}
	// И он должен реально работать после round-trip через диск.
	blob, _ := Encrypt([]byte("проверка"), id.Public())
	if _, err := Decrypt(blob, loaded); err != nil {
		t.Errorf("ключ с диска не расшифровал: %v", err)
	}
}

// Перезапись ключа = потеря доступа ко всему, что им зашифровано. Молча этого
// делать нельзя ни при каких обстоятельствах.
func TestSaveIdentity_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	first, _ := Generate()
	if err := SaveIdentity(dir, first); err != nil {
		t.Fatalf("первое сохранение: %v", err)
	}

	second, _ := Generate()
	if err := SaveIdentity(dir, second); err == nil {
		t.Fatal("перезапись существующего ключа прошла молча")
	}

	// Старый ключ должен остаться нетронутым.
	loaded, err := LoadIdentity(dir)
	if err != nil {
		t.Fatalf("загрузка после неудачной перезаписи: %v", err)
	}
	if loaded.String() != first.String() {
		t.Error("ключ на диске подменился, хотя перезапись была отвергнута")
	}
}

func TestLoadOrGenerate(t *testing.T) {
	dir := t.TempDir()

	id1, created, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("первый вызов: %v", err)
	}
	if !created {
		t.Error("первый вызов должен сообщить, что ключ создан — иначе пользователь не увидит предупреждения")
	}

	id2, created, err := LoadOrGenerate(dir)
	if err != nil {
		t.Fatalf("второй вызов: %v", err)
	}
	if created {
		t.Error("второй вызов не должен создавать новый ключ")
	}
	if id1.String() != id2.String() {
		t.Error("вернулся другой ключ — данные, зашифрованные первым, стали бы нечитаемы")
	}
}

func TestParse_RejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "не ключ", "age1", "AGE-SECRET-KEY-1"} {
		if _, err := ParseIdentity(s); err == nil {
			t.Errorf("ParseIdentity(%q) принял мусор", s)
		}
		if _, err := ParseRecipient(s); err == nil {
			t.Errorf("ParseRecipient(%q) принял мусор", s)
		}
	}
}

// Публичный ключ можно показывать, приватный — нет. Проверяем, что мы их не
// перепутали: у age разные префиксы.
func TestKeyFormats(t *testing.T) {
	id, _ := Generate()
	if !strings.HasPrefix(id.Public().String(), "age1") {
		t.Errorf("публичный ключ должен начинаться с age1: %s", id.Public())
	}
	if !strings.HasPrefix(id.String(), "AGE-SECRET-KEY-1") {
		t.Error("приватный ключ должен начинаться с AGE-SECRET-KEY-1")
	}
	if strings.Contains(id.Public().String(), "SECRET") {
		t.Error("в публичном ключе оказался секрет")
	}
}
