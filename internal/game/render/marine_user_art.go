package render

import "strings"

// Четыре независимых вида (без склейки колонок).
// Легенда символов:
// H = Каска/Голова, M = Бронежилет, A = Руки, W = Оружие, L = Ноги, B = Ботинки

// Лицом к камере («на меня прям»).
const marineAsciiFront = `
       .:HHH:.           
      .:HHHHH:.          
      .-HHHHH=.          
    ..AMMMMMMMA..      
  --AAAMMMMMMMAAAW+-
  =AAAMMMMMMMAAAWWWW
 WWWWW-WAAAAAAW-WWWW
     WWW---WAAAAA--=
        :=LLLLLLLL=:
        .:+LLLLLLL+:.    
        .-+LLLLLL+:.    
         .-LLLLLLLL:.    
         :LLLLLLLL-..    
      ..:LLLLLLLL-.      
     ..=LLLLLLLL:       
   .:LLLLLLLLLLL=.       
  .:LLLLLLLLLLL+.        
  .:LLLLLLLLLL:        
   ..-LLLLLLLL+.       
       .:BBB::BBB:       
         .BBB-BBB-       
         ..BB-BBB:       
      ..:-BBBBBBB:       
      .:--:.BBBBB=.      
         .-BBBBBB-. `

// Спиной к камере.
const marineAsciiBack = `
		.-HH-..
       .HHHHH-.
    ...-AMMMMA....
  ..-AAAMMMMMMAA-.
.WAAAAAMMMMMMMMAA.
  .WAAAAAMMMMAA=-..
  .WAAAAAMMMM..
     :AMMMMMM-.
   :AMMMMMMMMA.
  ..AMMMMMMMMA:.
    .:-LLLLLL-..
  .--LLLLLLLLL:.
   .:LLLLLLLLL+.
    :LLLLLLLLLL-..
   .:LLLLLLLLLLLL=.
 ..LLLL-.-LLLLLLL=.
    .LLLL-..LLLLL:.
   .:LLLL:..-LLLL-.
 .-LLL:.. :LLL=..
   .-BB-.   :BBB-.
   .-BB:.   .-BBB..
   .-BB.    .-BB:.
    :BB.     -BB:.
   .:BB=.   .-BBB-.
    :-=..   .=BB=-.`

// Профиль влево (кадр 2; кадр 6 = зеркало).
const marineAsciiSideLeft = `
            ..-HHH-.
            .-HHHHH-.
       ... ..-MHHHHH-
.::-WWWWWWWWWAAMMAA-.
     ..=WWWWWWWWWMAAA++-
       .-WWW-WW-WAAA--:
       .-WWWAAAMMAAAAA=-
        ..::-WWW-WAAAA=-
           .:AMMMMMMMMA=:
            .=MMMMMMMMMA:.
           :LLLLLLLLLL=.
           .+LLLLLLLLL:.
         .-LLLLLLLLLL-.
       ..LLLLLLLLLLLL:
        .:LLLLL--LLLLLL.
        .-LLL=:.-LLLLL.
       ..LLL+:.     .-LLLL..
        .:LL...:L   LLL:.
       .:=LL-.    .=LL=.
       .:BBB+... :BBBB:.
         .BBB.    .-BBB:.
         .BBB.    .BBBB.
         .BB-.      . BB:.
       .:BBBB=.     .-BB.
       ..:-=-.     ..--.`

// «Вправо» от экрана — ¾ для кадров 1/7.
const marineAsciiQuarterRight = `  
     .:HHH-..
    .:HHHHHH:
.....MHHHHHH-....    ..
WWWWWWMMAAAMMMMAAA++=:::..
WWWWWWWWAAMMMMMM.A=..
WWWWWWWWWAMMMAMMAA=:
:WAAAAAAAAW===AW==:.
.+AAMMMMMM+:::::.
:+MMMMMMMMM:.
=+LLLLLLLL+.
::LLLLLL-.
.-LLLLLLLLL.
 ..LLLLLLLL:.
..:LLLLLLLL-.
.LLLLLLLLLL-.
 ..LLLLLLL.
   .-LLLLLLL=
   .+LLLLLL+-
  .BBBBBB:.
  :BBB-:BBB.
  .BBB:.BB-.
 .:BBB..BB-.
 .:BBB.-BBB:...
::BBBB==BBBBBB:
..-BBBBBB.`

// marineGlyphFromAscii — базовый парсинг.
func marineGlyphFromAscii(ch rune) (rune, uint32) {
	if ch == ' ' || ch == '\t' || ch == 0 {
		return 0, 0
	}
	c := asciiArtColor(ch)
	return ch, c
}

// marineTintByPos — теперь нам НЕ НУЖНО угадывать зону по float-координатам! 
// Каждый символ уже знает, какой он части тела принадлежит.
func marineTintByPos(ch rune, y, h, x, maxW int) (rune, uint32) {
	if ch == ' ' || ch == '\t' || ch == 0 {
		return 0, 0
	}
	return marineGlyphFromAscii(ch)
}

func flipMarineStringLinesH(lines []string) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		r := []rune(ln)
		for j, k := 0, len(r)-1; j < k; j, k = j+1, k-1 {
			r[j], r[k] = r[k], r[j]
		}
		out[i] = string(r)
	}
	return out
}

func linesFromMarineBlock(s string) []string {
	s = strings.TrimRight(s, "\n\r")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	if len(lines) > 0 {
		lines[0] = "\t" + lines[0]
	}
	return lines
}

func init() {
	marineFrontLines = linesFromMarineBlock(marineAsciiFront)
	marineBackLines = linesFromMarineBlock(marineAsciiBack)
	marineSideLeftLines = linesFromMarineBlock(marineAsciiSideLeft)
	marineQuarter1Lines = linesFromMarineBlock(marineAsciiQuarterRight)
	marineQuarter3Lines = flipMarineStringLinesH(marineQuarter1Lines)
}

// asciiArtColor — сопоставляет конкретную букву/символ с нужным цветом.
func asciiArtColor(ch rune) uint32 {
	switch ch {
	case 'H', 'h': // Шлем / Голова
		return RGBPacked(75, 165, 108)
	case 'M', 'm': // Бронежилет / Торс
		return RGBPacked(22, 138, 55)
	case 'A', 'a': // Руки
		return RGBPacked(98, 128, 108)
	case 'W', 'w': // Оружие
		return RGBPacked(44, 44, 50)
	case 'L', 'l': // Штаны / Ноги
		return RGBPacked(78, 74, 72)
	case 'B', 'b': // Ботинки
		return RGBPacked(62, 58, 56)
	
	// Оставляем серые оттенки для "теневых" спецсимволов и обводки
	case '=', '-', '_', '~', '^':
		return RGBPacked(125, 132, 118)
	case '+', '|':
		return RGBPacked(145, 150, 138)
	case ':', ';', ',':
		return RGBPacked(95, 88, 78)
	case '.':
		return RGBPacked(72, 68, 62)
	default:
		return RGBPacked(118, 124, 112)
	}
}