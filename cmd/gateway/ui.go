package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// sshSafe убирает управляющие символы из строк, показываемых в ANSI-баннерах.
func sshSafe(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\x1b' || r == '\r' {
			continue
		}
		if r < 32 && r != '\t' && r != '\n' {
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 96 {
		return b.String()[:96] + "…"
	}
	return b.String()
}

func redGradient(text string, r, g, b int) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, text)
}

func drawRedGradientBanner(w io.Writer) {
	banner := []string{
		"",
		"  ███████╗███████╗██╗  ██╗      █████╗ ██████╗ ███████╗███╗   ██╗ █████╗ ",
		"  ██╔════╝██╔════╝██║  ██║     ██╔══██╗██╔══██╗██╔════╝████╗  ██║██╔══██╗",
		"  ███████╗███████╗███████║     ███████║██████╔╝█████╗  ██╔██╗ ██║███████║",
		"  ╚════██║╚════██║██╔══██║     ██╔══██║██╔══██╗██╔══╝  ██║╚██╗██║██╔══██║",
		"  ███████║███████║██║  ██║     ██║  ██║██║  ██║███████╗██║ ╚████║██║  ██║",
		"  ╚══════╝╚══════╝╚═╝  ╚═╝     ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═══╝╚═╝  ╚═╝",
		"",
		"  ╔═══════════════════════════════════════════════════════════════╗",
		"  ║         МНОГОПОЛЬЗОВАТЕЛЬСКАЯ БОЕВАЯ АРЕНА (SSH)             ║",
		"  ╚═══════════════════════════════════════════════════════════════╝",
		"",
	}
	for i, line := range banner {
		r := 255 - (i * 10)
		if r < 150 {
			r = 150
		}
		io.WriteString(w, redGradient(line+"\n", r, 0, 0))
		time.Sleep(22 * time.Millisecond)
	}
}

func drawNicknamePrompt(w io.Writer) {
	io.WriteString(w, "\n")
	for i := 0; i < 3; i++ {
		r := 255 - (i * 25)
		io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", r, 0, 0))
		time.Sleep(18 * time.Millisecond)
	}
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 230, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;80;80m▶ ВВЕДИТЕ ПОЗЫВНОЙ:\x1b[0m")
	io.WriteString(w, redGradient("                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 200, 0, 0))
	io.WriteString(w, "\n  \x1b[38;2;255;120;120m> \x1b[0m\x1b[?25h")
}

func drawModeMenu(w io.Writer) {
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║                    ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;100;100m* ВЫБЕРИТЕ РЕЖИМ *\x1b[0m")
	io.WriteString(w, redGradient("                    ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ╠═══════════════════════════════════════════════════════════╣\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║    ", 230, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m[1]\x1b[0m \x1b[38;2;255;255;255mСОЗДАТЬ НОВУЮ АРЕНУ\x1b[0m")
	io.WriteString(w, redGradient("                        ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 220, 0, 0))
	io.WriteString(w, redGradient("  ║    ", 220, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m[2]\x1b[0m \x1b[38;2;255;255;255mПРИСОЕДИНИТЬСЯ К АРЕНЕ\x1b[0m")
	io.WriteString(w, redGradient("                     ║\n", 220, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 210, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 200, 0, 0))
	io.WriteString(w, "\n  \x1b[38;2;255;120;120m▶ ВЫБЕРИТЕ [1/2]: \x1b[0m\x1b[?25h")
}

func drawRoomIDPrompt(w io.Writer) {
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;80;80m▶ ID АРЕНЫ:\x1b[0m")
	io.WriteString(w, redGradient("                                            ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 220, 0, 0))
	io.WriteString(w, "\n  \x1b[38;2;255;120;120m> \x1b[38;2;200;200;200m(по умолчанию: arena) \x1b[0m\x1b[?25h")
}

func drawMapChoicePrompt(w io.Writer) {
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║                   ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;100;100m* ВЫБОР КАРТЫ *\x1b[0m")
	io.WriteString(w, redGradient("                      ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ╠═══════════════════════════════════════════════════════════╣\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║    ", 230, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m[1]\x1b[0m \x1b[38;2;255;255;255mcorridor5\x1b[0m \x1b[38;2;150;150;150m— коридор, 5 комнат\x1b[0m")
	io.WriteString(w, redGradient("              ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║    ", 220, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m[2]\x1b[0m \x1b[38;2;255;255;255mсвой map_id\x1b[0m \x1b[38;2;150;150;150m— ввести ниже\x1b[0m")
	io.WriteString(w, redGradient("                  ║\n", 220, 0, 0))
	io.WriteString(w, redGradient("  ║    ", 215, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m[3]\x1b[0m \x1b[38;2;255;255;255mСВОЯ КАРТА\x1b[0m \x1b[38;2;150;150;150m— редактор в SSH\x1b[0m")
	io.WriteString(w, redGradient("               ║\n", 215, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 210, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 200, 0, 0))
	io.WriteString(w, "\n  \x1b[38;2;255;120;120m▶ [1] [2] или [3], затем Enter: \x1b[0m\x1b[?25h")
}

func drawMapEditorIntro(w io.Writer) {
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║           ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;100;100m* РЕДАКТОР КАРТЫ (SSH) *\x1b[0m")
	io.WriteString(w, redGradient("            ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ╠═══════════════════════════════════════════════════════════╣\n", 235, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 230, 0, 0))
	io.WriteString(w, "\x1b[38;2;200;200;200mКрасный фон · — пол, # — стена; рамка всегда стена.\x1b[0m")
	io.WriteString(w, redGradient("   ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 225, 0, 0))
	io.WriteString(w, "\x1b[38;2;200;200;200mw a s d — курсор, t — стена, p — точка спавна\x1b[0m")
	io.WriteString(w, redGradient("              ║\n", 225, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 220, 0, 0))
	io.WriteString(w, "\x1b[38;2;200;200;200mПосле каждой команды (w, a, s, d…) нажмите Enter.\x1b[0m")
	io.WriteString(w, redGradient("       ║\n", 220, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 218, 0, 0))
	io.WriteString(w, "\x1b[38;2;200;200;200mdone или export — готово; quit — отмена\x1b[0m")
	io.WriteString(w, redGradient("              ║\n", 218, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 210, 0, 0))
	io.WriteString(w, "\n")
}

func drawCustomMapPrompt(w io.Writer) {
	io.WriteString(w, "\n  \x1b[38;2;255;120;120m▶ map_id (например имя JSON без пути): \x1b[0m\x1b[?25h")
}

func drawConnectingBanner(w io.Writer) {
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║              ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;100;100m* ПОДКЛЮЧЕНИЕ К СЕРВЕРУ *\x1b[0m")
	io.WriteString(w, redGradient("              ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 220, 0, 0))
	io.WriteString(w, "\n  ")
}

func runConnectingSpinner(w io.Writer) {
	chars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	for i := 0; i < 10; i++ {
		io.WriteString(w, "\r  ")
		io.WriteString(w, redGradient(chars[i%len(chars)]+" ", 255, 0, 0))
		io.WriteString(w, "\x1b[38;2;255;150;150mУстановка соединения...\x1b[0m")
		time.Sleep(72 * time.Millisecond)
	}
	io.WriteString(w, "\n")
}

func drawManagerError(w io.Writer, errMsg string) {
	msg := sshSafe(errMsg)
	if len(msg) > 52 {
		msg = msg[:52] + "…"
	}
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║                  ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;50;50m* ОШИБКА ПОДКЛЮЧЕНИЯ *\x1b[0m")
	io.WriteString(w, redGradient("                  ║\n", 240, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 230, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;180;180m"+msg+"\x1b[0m")
	io.WriteString(w, redGradient("                                  ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 210, 0, 0))
}

func drawSessionSuccess(w io.Writer, created bool, name, roomID string) {
	name = sshSafe(name)
	roomID = sshSafe(roomID)
	io.WriteString(w, "\x1b[2J\x1b[H\n\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 245, 0, 0))
	if created {
		io.WriteString(w, redGradient("  ║              ", 235, 0, 0))
		io.WriteString(w, "\x1b[38;2;100;255;100m* АРЕНА СОЗДАНА *\x1b[0m")
		io.WriteString(w, redGradient("               ║\n", 235, 0, 0))
	} else {
		io.WriteString(w, redGradient("  ║                ", 235, 0, 0))
		io.WriteString(w, "\x1b[38;2;100;255;100m* ПОДКЛЮЧЕНО *\x1b[0m")
		io.WriteString(w, redGradient("                  ║\n", 235, 0, 0))
	}
	io.WriteString(w, redGradient("  ║                                                           ║\n", 230, 0, 0))
	io.WriteString(w, redGradient("  ╠═══════════════════════════════════════════════════════════╣\n", 225, 0, 0))
	io.WriteString(w, redGradient("  ║         ", 215, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;200;200mПозывной:\x1b[0m \x1b[38;2;255;255;100m"+name+"\x1b[0m")
	io.WriteString(w, redGradient("                                  ║\n", 215, 0, 0))
	io.WriteString(w, redGradient("  ║         ", 210, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;200;200mID арены:\x1b[0m \x1b[38;2;255;255;100m"+roomID+"\x1b[0m")
	io.WriteString(w, redGradient("                               ║\n", 210, 0, 0))
	io.WriteString(w, redGradient("  ║                                                           ║\n", 205, 0, 0))
	io.WriteString(w, redGradient("  ╠═══════════════════════════════════════════════════════════╣\n", 200, 0, 0))
	io.WriteString(w, redGradient("  ║              ", 190, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;150;150mНАЖМИТЕ ENTER — ВОЙТИ В БОЙ\x1b[0m")
	io.WriteString(w, redGradient("             ║\n", 190, 0, 0))
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 180, 0, 0))
	io.WriteString(w, "\n\n")
	for i := 0; i < 3; i++ {
		io.WriteString(w, "\r  ")
		io.WriteString(w, redGradient("              >> ГОТОВ К БОЮ <<", 255, 50, 50))
		time.Sleep(280 * time.Millisecond)
		io.WriteString(w, "\r  ")
		io.WriteString(w, redGradient("              >> ГОТОВ К БОЮ <<", 180, 0, 0))
		time.Sleep(280 * time.Millisecond)
	}
	io.WriteString(w, "\x1b[?25h\n")
}

func drawWireError(w io.Writer, title, detail string) {
	detail = sshSafe(detail)
	if len(detail) > 52 {
		detail = detail[:52] + "…"
	}
	io.WriteString(w, "\n\n")
	io.WriteString(w, redGradient("  ╔═══════════════════════════════════════════════════════════╗\n", 255, 0, 0))
	io.WriteString(w, redGradient("  ║  ", 240, 0, 0))
	io.WriteString(w, "\x1b[38;2;255;60;60m"+sshSafe(title)+"\x1b[0m")
	io.WriteString(w, redGradient("                                        ║\n", 240, 0, 0))
	if detail != "" {
		io.WriteString(w, redGradient("  ║  ", 230, 0, 0))
		io.WriteString(w, "\x1b[38;2;255;180;180m"+detail+"\x1b[0m")
		io.WriteString(w, redGradient("                                  ║\n", 230, 0, 0))
	}
	io.WriteString(w, redGradient("  ╚═══════════════════════════════════════════════════════════╝\n", 220, 0, 0))
}
