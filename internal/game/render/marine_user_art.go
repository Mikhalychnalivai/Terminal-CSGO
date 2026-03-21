package render

import "strings"

// Четыре независимых вида (без склейки колонок — иначе regex ломается на разной ширине строк).
// Порядок для движка: front (0), back (4), sideL (2), quarter (1/3 + зеркало) — см. BuildBuiltinMarine8.

// Лицом к камере («на меня прям»).
const marineAsciiFront = `
       .:---:.           
      .:=====:.          
      .-==+++=.          
    ..=###%++=:..      
  --=++*******+++*+-
  =++*##**++++++====
 =====-=++++++++*+====
     ===---=+++++--=
        :=********=:
        .:+*******+:.    
        .-+******+:.    
         .-++++==+=:.    
         :========-..    
      ..:========-.      
     ..=+========:       
   .:++*+++*++=.       
  .:+***=+***+.        
  .:++=+****:        
   ..-**+=:+*++.       
       .:===::===:       
         .=++-===-       
         ..=+=-++:       
      ..:-+++++++:       
      .:--:.-++++=.      
         .-++++==-. `

// Спиной к камере.
const marineAsciiBack = `
		.-==-..
       .====+-.
    ...-+##*+-....
  ..-++========-.
.=****+=========-.
  .+****+===+--=-..
  .=****+==++..
     :+****++-.
   :+********=.
  ..=*******+:.
    .:-+++*++-..
  .--====+==++:.
   .:=========+.
    :=========++-..
   .:========+***=.
 ..====-.-+****=.
    .=***-..+***+:.
   .:+*++:..-+++*-.
 .-+++:.. :+++=..
   .-==-.   :===-.
   .-==:.   .-==-..
   .-++.    .-++:.
    :++.     -+=:.
   .:++=.   .-++=-.
    :-=..   .=+==-.`

// Профиль влево (кадр 2; кадр 6 = зеркало в коде).
const marineAsciiSideLeft = `
            ..-=++-.
            .-+====-.
       ... ..-%*==++-
.::-*##+****+****+==-.
     ..=+***********+++-
       .-==--==-=+++--:
       .-==+*****+****=-
        ..::-===-===*==-
           .:=***********=:
            .=**********+:.
           :+*+*+++++*=.
           .+=========:.
         .-==========-.
       ..============+:
        .:=====--===+*+.
        .-+===:.-+++++.
       ..+**+:.     .-***=..
        .:++...:+   +++:.
       .:=++-.    .=++=.
       .:+**+... :++++:.
         .===.    .-==+:.
         .=+=.    .=+==.
         .=+-.      . =+:.
       .:+++=.     .-++.
       ..:-=-.           ..--.`

// «Вправо» от экрана — ¾ для кадров 1/7; кадры 3/5 = зеркало по горизонтали.
const marineAsciiQuarterRight = `  
     .:===-..
    .:+=====:
.....=+++===-....    ..
-==++*%%%#**####++++=:::..
-=======+*******.+=..
-==++++++=***=*#===:
:++*******+===++==:.
.+********+:::::.
:+*********:.
=+********+.
::=+++++-.
.-=+*+++===.
 ..+*+=====:.
..:+*======-.
.=+****+===-.
 ..+++++++.
   .-*******=
   .+******+-
  .=+++++:.
  :===-:===.
  .===:.++-.
 .:++-..++-.
 .:++-.-++=:...
::=+++==+++++=:
..-==+++=.`

// marineGlyphFromAscii — символы из пользовательского аскии (= * # % + - : . и т.д.).
func marineGlyphFromAscii(ch rune) (rune, uint32) {
	if ch == ' ' || ch == '\t' || ch == 0 {
		return 0, 0
	}
	c := asciiArtColor(ch)
	return ch, c
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

// linesFromMarineBlock — первая строка «съехала»: добавляем одну табуляцию, как просили.
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

func asciiArtColor(ch rune) uint32 {
	switch ch {
	case '#', '%':
		return RGBPacked(40, 95, 42)
	case '*':
		return RGBPacked(55, 130, 52)
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
