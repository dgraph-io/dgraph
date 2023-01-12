package lex

import (
	"reflect"
	"testing"
)

// func TestItem_Errorf(t *testing.T) {
// 	type fields struct {
// 		Typ    ItemType
// 		Val    string
// 		line   int
// 		column int
// 	}
// 	type args struct {
// 		format string
// 		args   []interface{}
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			i := Item{
// 				Typ:    tt.fields.Typ,
// 				Val:    tt.fields.Val,
// 				line:   tt.fields.line,
// 				column: tt.fields.column,
// 			}
// 			if err := i.Errorf(tt.args.format, tt.args.args...); (err != nil) != tt.wantErr {
// 				t.Errorf("Item.Errorf() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func TestItem_String(t *testing.T) {
// 	type fields struct {
// 		Typ    ItemType
// 		Val    string
// 		line   int
// 		column int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   string
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			i := Item{
// 				Typ:    tt.fields.Typ,
// 				Val:    tt.fields.Val,
// 				line:   tt.fields.line,
// 				column: tt.fields.column,
// 			}
// 			if got := i.String(); got != tt.want {
// 				t.Errorf("Item.String() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestLexer_NewIterator(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   *ItemIterator
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if got := l.NewIterator(); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("Lexer.NewIterator() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Errorf(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	type args struct {
// 		format string
// 		args   []interface{}
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			if err := p.Errorf(tt.args.format, tt.args.args...); (err != nil) != tt.wantErr {
// 				t.Errorf("ItemIterator.Errorf() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Next(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			if got := p.Next(); got != tt.want {
// 				t.Errorf("ItemIterator.Next() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Item(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   Item
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			if got := p.Item(); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("ItemIterator.Item() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Prev(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			if got := p.Prev(); got != tt.want {
// 				t.Errorf("ItemIterator.Prev() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Restore(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	type args struct {
// 		pos int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			p.Restore(tt.args.pos)
// 		})
// 	}
// }

// func TestItemIterator_Save(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   int
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			if got := p.Save(); got != tt.want {
// 				t.Errorf("ItemIterator.Save() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_Peek(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	type args struct {
// 		num int
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    []Item
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			got, err := p.Peek(tt.args.num)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("ItemIterator.Peek() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("ItemIterator.Peek() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestItemIterator_PeekOne(t *testing.T) {
// 	type fields struct {
// 		l   *Lexer
// 		idx int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		want   Item
// 		want1  bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			p := &ItemIterator{
// 				l:   tt.fields.l,
// 				idx: tt.fields.idx,
// 			}
// 			got, got1 := p.PeekOne()
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("ItemIterator.PeekOne() got = %v, want %v", got, tt.want)
// 			}
// 			if got1 != tt.want1 {
// 				t.Errorf("ItemIterator.PeekOne() got1 = %v, want %v", got1, tt.want1)
// 			}
// 		})
// 	}
// }

// func TestLexer_Reset(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		input string
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.Reset(tt.args.input)
// 		})
// 	}
// }

// func TestLexer_ValidateResult(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if err := l.ValidateResult(); (err != nil) != tt.wantErr {
// 				t.Errorf("Lexer.ValidateResult() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func TestLexer_Run(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		f StateFn
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 		want   *Lexer
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if got := l.Run(tt.args.f); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("Lexer.Run() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestLexer_Errorf(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		format string
// 		args   []interface{}
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 		want   StateFn
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if got := l.Errorf(tt.args.format, tt.args.args...); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("Lexer.Errorf() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestLexer_Emit(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		t ItemType
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.Emit(tt.args.t)
// 		})
// 	}
// }

// func TestLexer_pushWidth(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		width int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.pushWidth(tt.args.width)
// 		})
// 	}
// }

// func TestLexer_Next(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	tests := []struct {
// 		name       string
// 		fields     fields
// 		wantResult rune
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if gotResult := l.Next(); gotResult != tt.wantResult {
// 				t.Errorf("Lexer.Next() = %v, want %v", gotResult, tt.wantResult)
// 			}
// 		})
// 	}
// }

// func TestLexer_Backup(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.Backup()
// 		})
// 	}
// }

// need write more tests from other positions (fields.Pos)
func TestLexer_Peek(t *testing.T) {
	type fields struct {
		Input      string
		Start      int
		Pos        int
		Width      int
		widthStack []*RuneWidth
		items      []Item
		Depth      int
		BlockDepth int
		ArgDepth   int
		Mode       StateFn
		Line       int
		Column     int
	}
	tests := []struct {
		name   string
		fields fields
		want   rune
	}{
		{
			name: "TestLexer_Peek 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			want: 't',
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lexer{
				Input:      tt.fields.Input,
				Start:      tt.fields.Start,
				Pos:        tt.fields.Pos,
				Width:      tt.fields.Width,
				widthStack: tt.fields.widthStack,
				items:      tt.fields.items,
				Depth:      tt.fields.Depth,
				BlockDepth: tt.fields.BlockDepth,
				ArgDepth:   tt.fields.ArgDepth,
				Mode:       tt.fields.Mode,
				Line:       tt.fields.Line,
				Column:     tt.fields.Column,
			}
			if got := l.Peek(); got != tt.want {
				t.Errorf("Lexer.Peek() = %v, want %v", got, tt.want)
			}
		})
	}
}

// need write more tests from other positions (fields.Pos)
func TestLexer_PeekTwo(t *testing.T) {
	type fields struct {
		Input      string
		Start      int
		Pos        int
		Width      int
		widthStack []*RuneWidth
		items      []Item
		Depth      int
		BlockDepth int
		ArgDepth   int
		Mode       StateFn
		Line       int
		Column     int
	}
	tests := []struct {
		name   string
		fields fields
		want   []rune
	}{
		{
			name: "TestLexer_PeekTwo 1",
			fields: fields{
				Input:      "test string",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			want: []rune{'t', 'e'},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lexer{
				Input:      tt.fields.Input,
				Start:      tt.fields.Start,
				Pos:        tt.fields.Pos,
				Width:      tt.fields.Width,
				widthStack: tt.fields.widthStack,
				items:      tt.fields.items,
				Depth:      tt.fields.Depth,
				BlockDepth: tt.fields.BlockDepth,
				ArgDepth:   tt.fields.ArgDepth,
				Mode:       tt.fields.Mode,
				Line:       tt.fields.Line,
				Column:     tt.fields.Column,
			}
			if got := l.PeekTwo(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Lexer.PeekTwo() = %v, want %v", got, tt.want)
			}
		})
	}
}

// need to understand this func*****
func TestLexer_moveStartToPos(t *testing.T) {
	type fields struct {
		Input      string
		Start      int
		Pos        int
		Width      int
		widthStack []*RuneWidth
		items      []Item
		Depth      int
		BlockDepth int
		ArgDepth   int
		Mode       StateFn
		Line       int
		Column     int
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "TestLexer_moveStartToPos 1",
			fields: fields{
				Input:      "This\n a simple\n test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
		},
		{
			name: "TestLexer_moveStartToPos 2",
			fields: fields{
				Input:      " ",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lexer{
				Input:      tt.fields.Input,
				Start:      tt.fields.Start,
				Pos:        tt.fields.Pos,
				Width:      tt.fields.Width,
				widthStack: tt.fields.widthStack,
				items:      tt.fields.items,
				Depth:      tt.fields.Depth,
				BlockDepth: tt.fields.BlockDepth,
				ArgDepth:   tt.fields.ArgDepth,
				Mode:       tt.fields.Mode,
				Line:       tt.fields.Line,
				Column:     tt.fields.Column,
			}
			l.moveStartToPos()
		})
	}
}

// need to understand this func*****
func TestLexer_Ignore(t *testing.T) {
	type fields struct {
		Input      string
		Start      int
		Pos        int
		Width      int
		widthStack []*RuneWidth
		items      []Item
		Depth      int
		BlockDepth int
		ArgDepth   int
		Mode       StateFn
		Line       int
		Column     int
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "TestLexer_Ignore 1",
			fields: fields{
				Input:      "Test comments",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lexer{
				Input:      tt.fields.Input,
				Start:      tt.fields.Start,
				Pos:        tt.fields.Pos,
				Width:      tt.fields.Width,
				widthStack: tt.fields.widthStack,
				items:      tt.fields.items,
				Depth:      tt.fields.Depth,
				BlockDepth: tt.fields.BlockDepth,
				ArgDepth:   tt.fields.ArgDepth,
				Mode:       tt.fields.Mode,
				Line:       tt.fields.Line,
				Column:     tt.fields.Column,
			}
			l.Ignore()
		})
	}
}

// func TestLexer_AcceptRun(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		c CheckRune
// 	}
// 	tests := []struct {
// 		name       string
// 		fields     fields
// 		args       args
// 		wantLastr  rune
// 		wantValidr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			gotLastr, gotValidr := l.AcceptRun(tt.args.c)
// 			if gotLastr != tt.wantLastr {
// 				t.Errorf("Lexer.AcceptRun() gotLastr = %v, want %v", gotLastr, tt.wantLastr)
// 			}
// 			if gotValidr != tt.wantValidr {
// 				t.Errorf("Lexer.AcceptRun() gotValidr = %v, want %v", gotValidr, tt.wantValidr)
// 			}
// 		})
// 	}
// }

// func TestLexer_AcceptRunRec(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		c CheckRuneRec
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.AcceptRunRec(tt.args.c)
// 		})
// 	}
// }

// func TestLexer_AcceptUntil(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		c CheckRune
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.AcceptUntil(tt.args.c)
// 		})
// 	}
// }

// func TestLexer_AcceptRunTimes(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		c     CheckRune
// 		times int
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 		want   int
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if got := l.AcceptRunTimes(tt.args.c, tt.args.times); got != tt.want {
// 				t.Errorf("Lexer.AcceptRunTimes() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

// func TestLexer_IgnoreRun(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	type args struct {
// 		c CheckRune
// 	}
// 	tests := []struct {
// 		name   string
// 		fields fields
// 		args   args
// 	}{
// 		{
// 			name: "TestLexer_IgnoreRun 1",
// 			fields: fields{
// 				Input:      "Test string",
// 				Start:      0,
// 				Pos:        0,
// 				Width:      0,
// 				widthStack: []*RuneWidth{},
// 				items:      []Item{},
// 				Depth:      0,
// 				BlockDepth: 0,
// 				ArgDepth:   0,
// 				Line:       0,
// 				Column:     0,
// 			},
// 			args: args{},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			l.IgnoreRun(tt.args.c)
// 		})
// 	}
// }

// use this example for iri_test.go
func TestLexer_IsEscChar(t *testing.T) {
	type fields struct {
		Input      string
		Start      int
		Pos        int
		Width      int
		widthStack []*RuneWidth
		items      []Item
		Depth      int
		BlockDepth int
		ArgDepth   int
		Mode       StateFn
		Line       int
		Column     int
	}
	type args struct {
		r rune
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "TestLexer_IsEscChar 1",
			fields: fields{
				Input:      "u",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{r: 'u'},
			want: true,
		},
		{
			name: "TestLexer_IsEscChar 2",
			fields: fields{
				Input:      "a",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{r: 'a'},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lexer{
				Input:      tt.fields.Input,
				Start:      tt.fields.Start,
				Pos:        tt.fields.Pos,
				Width:      tt.fields.Width,
				widthStack: tt.fields.widthStack,
				items:      tt.fields.items,
				Depth:      tt.fields.Depth,
				BlockDepth: tt.fields.BlockDepth,
				ArgDepth:   tt.fields.ArgDepth,
				Mode:       tt.fields.Mode,
				Line:       tt.fields.Line,
				Column:     tt.fields.Column,
			}
			if got := l.IsEscChar(tt.args.r); got != tt.want {
				t.Errorf("Lexer.IsEscChar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsEndOfLine(t *testing.T) {
	type args struct {
		r rune
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "TestIsEndOfLine 1",
			args: args{r: '\u000A'},
			want: true,
		},
		{
			name: "TestIsEndOfLine 2",
			args: args{r: '\u000D'},
			want: true,
		},
		{
			name: "TestIsEndOfLine 3",
			args: args{r: 't'},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEndOfLine(tt.args.r); got != tt.want {
				t.Errorf("IsEndOfLine() = %v, want %v", got, tt.want)
			}
		})
	}
}

// func TestLexer_LexQuotedString(t *testing.T) {
// 	type fields struct {
// 		Input      string
// 		Start      int
// 		Pos        int
// 		Width      int
// 		widthStack []*RuneWidth
// 		items      []Item
// 		Depth      int
// 		BlockDepth int
// 		ArgDepth   int
// 		Mode       StateFn
// 		Line       int
// 		Column     int
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		wantErr bool
// 	}{
// 		{
// 			name: "TestLexer_LexQuotedString 1",
// 			fields: fields{
// 				Input:      "string",
// 				Start:      0,
// 				Pos:        0,
// 				Width:      0,
// 				widthStack: []*RuneWidth{},
// 				items:      []Item{},
// 				Depth:      0,
// 				BlockDepth: 0,
// 				ArgDepth:   0,
// 				Line:       0,
// 				Column:     0,
// 			},
// 			wantErr: true,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			l := &Lexer{
// 				Input:      tt.fields.Input,
// 				Start:      tt.fields.Start,
// 				Pos:        tt.fields.Pos,
// 				Width:      tt.fields.Width,
// 				widthStack: tt.fields.widthStack,
// 				items:      tt.fields.items,
// 				Depth:      tt.fields.Depth,
// 				BlockDepth: tt.fields.BlockDepth,
// 				ArgDepth:   tt.fields.ArgDepth,
// 				Mode:       tt.fields.Mode,
// 				Line:       tt.fields.Line,
// 				Column:     tt.fields.Column,
// 			}
// 			if err := l.LexQuotedString(); (err != nil) != tt.wantErr {
// 				t.Errorf("Lexer.LexQuotedString() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }
