package editor

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"

	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

type EditorConfig struct {
	ScrollEnabled bool
	Username      string
}

type Editor struct {
	Text   []rune
	Cursor int
	Width  int
	Height int
	ColOff int
	RowOff int

	ShowMsg    bool
	StatusMsg  string
	StatusChan chan string
	StatusMu   sync.Mutex

	Username string
	Users    []string
	UsersPos map[string]CursorColPos

	ScrollEnabled bool
	IsConnected   bool
	DrawChan      chan int
	mu            sync.RWMutex
}

type CursorColPos struct {
	Pos int
	Col termbox.Attribute
}

var userColors = []termbox.Attribute{
	termbox.ColorGreen,
	termbox.ColorYellow,
	termbox.ColorBlue,
	termbox.ColorMagenta,
	termbox.ColorCyan,
	termbox.ColorLightYellow,
	termbox.ColorLightMagenta,
	termbox.ColorLightGreen,
	termbox.ColorLightRed,
	termbox.ColorRed,
}

func GetColorForUsername(username string, usernames []string) termbox.Attribute {
	sorted := make([]string, len(usernames))
	copy(sorted, usernames)
	sort.Strings(sorted)

	idx := 0
	for i, name := range sorted {
		if name == username {
			idx = i
			break
		}
	}

	return userColors[idx%len(userColors)]
}

func NewEditor(conf EditorConfig) *Editor {
	return &Editor{
		ScrollEnabled: conf.ScrollEnabled,
		StatusChan:    make(chan string, 100),
		DrawChan:      make(chan int, 10000),
		UsersPos:      make(map[string]CursorColPos),
		Username:      conf.Username,
	}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (e *Editor) GetText() []rune {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Text
}

func (e *Editor) SetText(text string) {
	e.mu.Lock()
	e.Text = []rune(text)
	e.mu.Unlock()
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (e *Editor) GetX() int {
	x, _ := e.calcXY(e.Cursor)
	return x
}

func (e *Editor) SetX(x int) {
	e.Cursor = x
}

func (e *Editor) GetY() int {
	_, y := e.calcXY(e.Cursor)
	return y
}

func (e *Editor) GetWidth() int {
	return e.Width
}

func (e *Editor) GetHeight() int {
	return e.Height
}

func (e *Editor) SetSize(w, h int) {
	e.Width = w
	e.Height = h
}

func (e *Editor) GetRowOff() int {
	return e.RowOff
}

func (e *Editor) GetColOff() int {
	return e.ColOff
}

func (e *Editor) IncRowOff(inc int) {
	e.RowOff += inc
}

func (e *Editor) IncColOff(inc int) {
	e.ColOff += inc
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (e *Editor) SendDraw() {
	e.DrawChan <- 1
}

func (e *Editor) Draw() {
	_ = termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)

	// draw cursor x position relative to row offset
	if cx-e.GetColOff() > 0 {
		cx -= e.GetColOff()
	}

	// draw cursor y position relative to row offset
	if cy-e.GetRowOff() > 0 {
		cy -= e.GetRowOff()
	}

	termbox.SetCursor(cx-1, cy-1)

	// find the starting and ending row of the termbox window.
	yStart := e.GetRowOff()
	yEnd := yStart + e.GetHeight() - 1 // -1 for StatusBar

	// find the starting ending column of the termbox window.
	xStart := e.GetColOff()

	x, y := 0, 0
	for i := 0; i < len(e.Text) && y < yEnd; i++ {
		if e.Text[i] == rune('\n') {
			x = 0
			y++
		} else {

			bg := termbox.ColorDefault
			for _, user := range e.UsersPos {
				if user.Pos == i {
					bg = user.Col
					break
				}
			}

			// Set cell content. setX and setY account for the window offset.
			setY := y - yStart
			setX := x - xStart
			termbox.SetCell(setX, setY, e.Text[i], termbox.ColorDefault, bg)

			// Update x by rune's width.
			x = x + runewidth.RuneWidth(e.Text[i])
		}
	}

	e.DrawStatusBar()
	termbox.Flush()
}

func (e *Editor) DrawStatusBar() {
	e.StatusMu.Lock()
	showMsg := e.ShowMsg
	e.StatusMu.Unlock()
	if showMsg {
		e.DrawStatusMsg()
	} else {
		e.DrawInfoBar()
	}

	// Render connection-indicator
	if e.IsConnected {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorGreen)
	} else {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorRed)
	}
}

func (e *Editor) DrawStatusMsg() {
	e.StatusMu.Lock()
	statusMsg := e.StatusMsg
	e.StatusMu.Unlock()
	for i, r := range []rune(statusMsg) {
		termbox.SetCell(i, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
	}
}

func (e *Editor) DrawInfoBar() {
	e.StatusMu.Lock()
	users := e.Users
	e.StatusMu.Unlock()

	e.mu.RLock()
	length := len(e.Text)
	e.mu.RUnlock()

	x := 0
	for _, user := range users {
		for _, r := range user {
			hash := fnv.New32a()
			hash.Write([]byte(user))
			color := GetColorForUsername(user, users)
			termbox.SetCell(x, e.Height-1, r, color, termbox.ColorDefault)
			x++
		}
		termbox.SetCell(x, e.Height-1, ' ', termbox.ColorDefault, termbox.ColorDefault)
		x++
	}

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)
	debugInfo := fmt.Sprintf(" x=%d, y=%d, cursor=%d, len(text)=%d", cx, cy, e.Cursor, length)

	for _, r := range debugInfo {
		termbox.SetCell(x, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
		x++
	}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (e *Editor) MoveCursor(x, y int) {
	if len(e.Text) == 0 && e.Cursor == 0 {
		return
	}
	// horizontally.
	newCursor := e.Cursor + x

	// vertically.
	if y > 0 {
		newCursor = e.calcCursorDown()
	}

	if y < 0 {
		newCursor = e.calcCursorUp()
	}

	if e.ScrollEnabled {
		cx, cy := e.calcXY(newCursor)

		rowStart := e.GetRowOff()
		rowEnd := e.GetRowOff() + e.GetHeight() - 1

		// scroll up
		if cy <= rowStart {
			e.IncRowOff(cy - rowStart - 1)
		}

		// scroll down
		if cy > rowEnd {
			e.IncRowOff(cy - rowEnd)
		}

		colStart := e.GetColOff()
		colEnd := e.GetColOff() + e.GetWidth()

		// scroll left
		if cx <= colStart {
			e.IncColOff(cx - (colStart + 1))
		}

		// scroll right
		if cx > colEnd {
			e.IncColOff(cx - colEnd)
		}
	}

	if newCursor > len(e.Text) {
		newCursor = len(e.Text)
	}

	if newCursor < 0 {
		newCursor = 0
	}
	e.mu.Lock()
	e.Cursor = newCursor
	e.mu.Unlock()
}

func (e *Editor) calcCursorUp() int {
	pos := e.Cursor
	offset := 0

	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--
	}

	if pos < 0 {
		pos = 0
	}

	start, end := pos, pos

	for start > 0 && e.Text[start] != '\n' {
		start--
	}

	if start == 0 {
		return 0
	}

	for end < len(e.Text) && e.Text[end] != '\n' {
		end++
	}

	prevStart := start - 1
	for prevStart >= 0 && e.Text[prevStart] != '\n' {
		prevStart--
	}

	offset += pos - start
	if offset <= start-prevStart {
		return prevStart + offset
	} else {
		return start
	}
}

func (e *Editor) calcCursorDown() int {
	pos := e.Cursor
	offset := 0

	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--
	}

	if pos < 0 {
		pos = 0
	}

	start, end := pos, pos

	for start > 0 && e.Text[start] != '\n' {
		start--
	}

	if start == 0 && e.Text[start] != '\n' {
		offset++
	}

	for end < len(e.Text) && e.Text[end] != '\n' {
		end++
	}

	if e.Text[pos] == '\n' && e.Cursor != 0 {
		end++
	}

	if end == len(e.Text) {
		return len(e.Text)
	}

	nextEnd := end + 1
	for nextEnd < len(e.Text) && e.Text[nextEnd] != '\n' {
		nextEnd++
	}

	offset += pos - start
	if offset < nextEnd-end {
		return end + offset
	} else {
		return nextEnd
	}
}

func (e *Editor) calcXY(index int) (int, int) {
	x := 1
	y := 1

	if index < 0 {
		return x, y
	}

	e.mu.RLock()
	length := len(e.Text)
	e.mu.RUnlock()

	if index > length {
		index = length
	}

	for i := 0; i < index; i++ {
		e.mu.RLock()
		r := e.Text[i]
		e.mu.RUnlock()
		if r == rune('\n') {
			x = 1
			y++
		} else {
			x = x + runewidth.RuneWidth(r)
		}
	}
	return x, y
}
