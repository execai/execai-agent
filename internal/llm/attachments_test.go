package llm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Пути к картинкам приходят на трёх ОС в трёх видах. Windows не поддерживался:
// панель прикладывала C:\Users\…\clipboard.png, разбор не находил ничего, и
// модель честно отвечала «я не вижу картинок» — это выглядело как «Kimi не
// умеет в картинки», хотя источник тут ни при чём.
func TestExtractImageAttachments_PathFlavours(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "shot.png")
	// 1x1 PNG
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 16))
	if err := os.WriteFile(img, png, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("реальный путь этой ОС в кавычках", func(t *testing.T) {
		_, images := ExtractImageAttachments(`что тут "` + img + `"`)
		if len(images) != 1 {
			t.Fatalf("картинка не подхвачена: %+v", images)
		}
	})

	// Форму windows-пути проверяем регуляркой, а не чтением файла: на Linux
	// такого файла нет, но распознаться он обязан — иначе панель на Windows
	// снова останется без картинок.
	t.Run("форма windows-пути распознаётся", func(t *testing.T) {
		for _, p := range []string{
			`"C:\Users\yz\AppData\Local\Temp\execai-attach\1787-clipboard.png"`,
			`"c:/Users/yz/Pictures/Снимок экрана.png"`,
			`"\\server\share\shot.jpeg"`,
		} {
			if !imageQuotedRE.MatchString(p) {
				t.Errorf("не распознан путь: %s", p)
			}
		}
	})

	t.Run("posix-путь без кавычек", func(t *testing.T) {
		if !imageUnquotedRE.MatchString(" /tmp/a/shot.png ") {
			t.Error("posix-путь без кавычек перестал распознаваться")
		}
	})

	t.Run("явный список файлов не зависит от разбора текста", func(t *testing.T) {
		content := BuildUserContentWithFiles("что на картинке", []string{img})
		blocks, ok := content.([]ContentBlock)
		if !ok {
			t.Fatalf("ожидались блоки, получено %T", content)
		}
		var imgs int
		for _, b := range blocks {
			if b.Type == "image_url" {
				imgs++
			}
		}
		if imgs != 1 {
			t.Fatalf("картинка из списка файлов не приложена: %+v", blocks)
		}
	})

	t.Run("дубликат из текста и списка прикладывается один раз", func(t *testing.T) {
		content := BuildUserContentWithFiles(`смотри "`+img+`"`, []string{img})
		blocks := content.([]ContentBlock)
		var imgs int
		for _, b := range blocks {
			if b.Type == "image_url" {
				imgs++
			}
		}
		if imgs != 1 {
			t.Fatalf("картинка приложена %d раза вместо одного", imgs)
		}
	})
	_ = runtime.GOOS
}
