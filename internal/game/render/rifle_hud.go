package render

import "strings"

// HUDRifleDamage — урон автомата на сервере (room.tryFireLocked) при weapon=HUDWeaponRifle.
const HUDRifleDamage = 33

// ASCII-арт АК для HUD (компактная «моделька»).
const rifleAsciiRaw = `
                                          #                                                         
                                        %}#+ #}>*                                                   
                                          %##<]#}%#%@%%                                             
                                            ]}}}}}}}}}}}[:                                           
                                            #%#%%[}}}}}}>#}-                                        
                                          %%#%%%#@#}}}}}}}%#[>                                    
                                         ##%%%%%#@%@%%%@}[%%%#})}                                 
                                        %%%%%##%%%%@%%%%%@@#}%%%%%#[#                              
                                       +)])[%#%%@@%@%%%#}%@%#%#}#%%%%#}##                           
                                      ><<<)))][%   %%%%@#%%%%%%#%##[%%%%%%%                        
                                     <<<<))))][   :%%%%%%%%%%%}%%%%%%#)#%##%>                      
                                   <)<<<<<)<)]}   %#%#%%%@%%%%%%@%%}%%%%%%#}%%                     
                                  )))<<<<<))][   -%##%%@%[[]%%%@%%%%%%#%%%@@%%%                   
                                ))<<<<<<<<))[   %%%%%%%@@@@@ @#%%%%%%%%%@%@[@@%%                 
                              <<<<<<<<)<))]]   #%%%%%%@@<)]))[%%%%%%%}}}}}}}}}}}}})               
                             <<<<<)<<<<<)))[     #%%#%@@ ]]])<<<))%%%%%}}}}}}}}}}}}#}:            
                           )<<<<<)<<<<<)))[      %%%%@@%   #}])<))<]]}%%}}}}}}}}}})}}}[[          
                         ))<)<<<<<<<<<))]]}      }%#@@%    )}]%])<)]]]][}}}}}}}}}}}}}}[[}})}       
                       <))<<<<))<<<<<))]][        %%%:      ][}@[]])<))][}}}}}}}}}}}}}}[}}}<[    
                     )))<)<)<<))<<<<))][[                   []]@%]])))<))]%@%%}}}}}}}}}}}}[}}#)} 
                   :))))<)<<)<)<<<<)]][[}                   *[}%#%}]))))))]%%%%}}}}}}}}}}}}}}}][##
                  ]))))))<<)<<<<<<)]][[}                    -}%%#%%}}[]]])]][[[#}}}}}}}}}}}}}}}}}[]
                 ])))))))<<<)<<<)))][[}                       %%#%%%@#}#[[]))]][}}}}}}}}}}}}}}}}}}}}}
`

// BuiltinRifleHUDFrames — 4 кадра для анимации огня (пока один и тот же кадр).
var BuiltinRifleHUDFrames []PistolHUDFrame

func init() {
	lines := trimAsciiBox(rifleAsciiRaw)
	if len(lines) == 0 {
		return
	}
	fr := cropFrameH(parseRifleHUDFrame(lines), 2, 2) // на 4 колонки уже (2+2)
	BuiltinRifleHUDFrames = []PistolHUDFrame{fr, fr, fr, fr}
}

// cropFrameH обрезает кадр слева/справа (симметрично уже́).
func cropFrameH(fr PistolHUDFrame, left, right int) PistolHUDFrame {
	rows := len(fr.Chars)
	if rows == 0 || left < 0 || right < 0 {
		return fr
	}
	w0 := len(fr.Chars[0])
	if left+right >= w0 {
		return fr
	}
	cw := w0 - left - right
	out := PistolHUDFrame{
		Chars: make([][]rune, rows),
		RGB:   make([][]uint32, rows),
	}
	for y := 0; y < rows; y++ {
		out.Chars[y] = make([]rune, cw)
		out.RGB[y] = make([]uint32, cw)
		for x := 0; x < cw; x++ {
			out.Chars[y][x] = fr.Chars[y][x+left]
			out.RGB[y][x] = fr.RGB[y][x+left]
		}
	}
	return out
}

func trimAsciiBox(s string) []string {
	s = strings.TrimRight(s, "\n\r")
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines) - 1
	for end > start && strings.TrimSpace(lines[end]) == "" {
		end--
	}
	if end < start {
		return nil
	}
	lines = lines[start : end+1]
	minPad := 1 << 30
	for _, ln := range lines {
		ln = strings.TrimRight(ln, " ")
		if ln == "" {
			continue
		}
		for i := 0; i < len(ln); i++ {
			if ln[i] != ' ' && ln[i] != '\t' {
				if i < minPad {
					minPad = i
				}
				break
			}
		}
	}
	if minPad > 1<<20 {
		minPad = 0
	}
	out := make([]string, len(lines))
	for i, ln := range lines {
		if len(ln) > minPad {
			out[i] = strings.TrimRight(ln[minPad:], " ")
		} else {
			out[i] = ""
		}
	}
	return out
}

func rifleHUDGlyph(ch rune) (rune, uint32) {
	if ch == ' ' || ch == '\t' || ch == 0 {
		return 0, 0
	}
	switch ch {
	case '#':
		// основной металл — blued / parkerized, очень тёмный чёрный с синим оттенком
		return ch, RGBPacked(20, 22, 28) // почти чёрный, лёгкий синий отлив

	case '%':
		// потёртый / основной металл (ресивер, ствол)
		return ch, RGBPacked(35, 38, 45) // тёмно-серый-металл

	case '@':
		// блик на металле — приглушённый, не яркий
		return ch, RGBPacked(70, 75, 85) // серо-металлический блик

	// Скобки — те же беж/коричневые, что у ASCII-пистолета (PistolHUDBeige / PistolHUDBrown), по шагам смешения.
	case '<':
		return ch, PistolHUDBrownBeigeBlend(0) // беж пистолета
	case '>':
		return ch, PistolHUDBrownBeigeBlend(1)
	case '[':
		return ch, PistolHUDBrownBeigeBlend(2)
	case ']':
		return ch, PistolHUDBrownBeigeBlend(3)
	case '(':
		return ch, PistolHUDBrownBeigeBlend(4)
	case ')':
		return ch, PistolHUDBrownBeigeBlend(5) // коричневый '#'

	case '{', '}':
		// дерево / приклад (тёмно-красновато-коричневое, "Russian plum")
		return ch, RGBPacked(70, 40, 30) // тёмно-вишнёвый / красновато-коричневый

	case '-':
		// планка / рельса (тёмный металл)
		return ch, RGBPacked(45, 48, 55)

	case '+':
		// крепёж / штифт (металл с лёгким блеском)
		return ch, RGBPacked(90, 85, 70) // тёплый серо-жёлтый металл

	case '*':
		// яркий блик у дульного среза (приглушённый)
		return ch, RGBPacked(140, 130, 110) // тёплый металлический отблеск

	case ':':
		// переход / средний тон дерева/металла
		return ch, RGBPacked(90, 60, 50) // средний коричнево-металлический

	default:
		// fallback — средний тёмный металл
		return ch, RGBPacked(40, 45, 55)
	}
}

func parseRifleHUDFrame(lines []string) PistolHUDFrame {
	h := len(lines)
	if h == 0 {
		return PistolHUDFrame{}
	}
	rows := make([][]rune, h)
	maxW := 0
	for i, line := range lines {
		rows[i] = []rune(line)
		if len(rows[i]) > maxW {
			maxW = len(rows[i])
		}
	}
	fr := PistolHUDFrame{
		Chars: make([][]rune, h),
		RGB:   make([][]uint32, h),
	}
	for y := 0; y < h; y++ {
		fr.Chars[y] = make([]rune, maxW)
		fr.RGB[y] = make([]uint32, maxW)
		for x := 0; x < maxW; x++ {
			var ch rune = ' '
			if x < len(rows[y]) {
				ch = rows[y][x]
			}
			r, c := rifleHUDGlyph(ch)
			fr.Chars[y][x] = r
			fr.RGB[y][x] = c
		}
	}
	return fr
}
