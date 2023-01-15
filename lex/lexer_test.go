package lex

import (
	"reflect"
	"testing"
)

// need understand more about it*****
func TestItem_Errorf(t *testing.T) {
	type fields struct {
		Typ    ItemType
		Val    string
		line   int
		column int
	}
	type args struct {
		format string
		args   []interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "TestItem_Errorf 1",
			fields: fields{
				Typ:    0,
				Val:    "test",
				line:   0,
				column: 0,
			},
			args: args{
				format: "",
				args:   []interface{}{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := Item{
				Typ:    tt.fields.Typ,
				Val:    tt.fields.Val,
				line:   tt.fields.line,
				column: tt.fields.column,
			}
			if err := i.Errorf(tt.args.format, tt.args.args...); (err != nil) != tt.wantErr {
				t.Errorf("Item.Errorf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestItem_StringEOF(t *testing.T) {
	type fields struct {
		Typ    ItemType
		Val    string
		line   int
		column int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{name: "TestItem_String 1", fields: fields{
			Typ:    0,
			Val:    "",
			line:   0,
			column: 0,
		}, want: "EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := Item{
				Typ:    tt.fields.Typ,
				Val:    tt.fields.Val,
				line:   tt.fields.line,
				column: tt.fields.column,
			}
			if got := i.String(); got != tt.want {
				t.Errorf("Item.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItem_String(t *testing.T) {
	type fields struct {
		Typ    ItemType
		Val    string
		line   int
		column int
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "TestItem_String 1",
			fields: fields{
				Typ:    5,
				Val:    "test",
				line:   1,
				column: 1,
			},
			want: "EOF",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := Item{
				Typ:    tt.fields.Typ,
				Val:    tt.fields.Val,
				line:   tt.fields.line,
				column: tt.fields.column,
			}
			if got := i.String(); got == tt.want {
				t.Errorf("Item.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
// 		{
// 			name:   "TestLexer_NewIterator 1",
// 			fields: fields{},
// 			want: &ItemIterator{
// 				l: &Lexer{
// 					Input:      "test",
// 					Start:      1,
// 					Pos:        1,
// 					Width:      0,
// 					widthStack: []*RuneWidth{},
// 					items: []Item{
// 						{
// 							Typ:    0,
// 							Val:    "",
// 							line:   0,
// 							column: 0,
// 						},
// 						{
// 							Typ:    0,
// 							Val:    "",
// 							line:   0,
// 							column: 0,
// 						},
// 					},
// 					Depth:      0,
// 					BlockDepth: 0,
// 					ArgDepth:   0,
// 					Line:       0,
// 					Column:     0,
// 				},
// 				idx: 1,
// 			},
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
// 			if got := l.NewIterator(); !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("Lexer.NewIterator() = %v, want %v", got, tt.want)
// 			}
// 		})
// 	}
// }

func TestItemIterator_Errorf(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	type args struct {
		format string
		args   []interface{}
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "TestItemIterator_Errorf 1",
			fields: fields{
				l:   &Lexer{},
				idx: 0,
			},
			args: args{
				format: "",
				args:   []interface{}{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if err := p.Errorf(tt.args.format, tt.args.args...); (err != nil) != tt.wantErr {
				t.Errorf("ItemIterator.Errorf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestItemIterator_Next(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "TestItemIterator_Next 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    5,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    5,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 0,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if got := p.Next(); got != tt.want {
				t.Errorf("ItemIterator.Next() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItemIterator_Item(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   Item
	}{
		{
			name: "TestItemIterator_Item 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 0,
			},
			want: Item{
				Typ:    0,
				Val:    "test",
				line:   0,
				column: 0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if got := p.Item(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ItemIterator.Item() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItemIterator_ItemFalse(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   Item
	}{
		{
			name: "TestItemIterator_Item 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: -1,
			},
			want: Item{
				Typ:    0,
				Val:    "test",
				line:   -1,
				column: -1,
			},
		},
		{
			name: "TestItemIterator_Item 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if got := p.Item(); reflect.DeepEqual(got, tt.want) {
				t.Errorf("ItemIterator.Item() = %v, want %v", got, tt.want)
			}
		})
	}
}

// test[ok] > trying more false cases*****
func TestItemIterator_Prev(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "TestItemIterator_Prev 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 1,
			},
			want: true,
		},
		{
			name: "TestItemIterator_Prev 2",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 0,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if got := p.Prev(); got != tt.want {
				t.Errorf("ItemIterator.Prev() = %v, want %v", got, tt.want)
			}
		})
	}
}

// understand more about it
func TestItemIterator_Restore(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	type args struct {
		pos int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "TestItemIterator_Restore 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 1,
			},
			args: args{
				pos: 0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			p.Restore(tt.args.pos)
		})
	}
}

// check conditions for test*****
func TestItemIterator_Save(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   int
	}{
		{
			name: "TestItemIterator_Save 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items:      []Item{},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 1,
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			if got := p.Save(); got != tt.want {
				t.Errorf("ItemIterator.Save() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItemIterator_Peek(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	type args struct {
		num int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []Item
		wantErr bool
	}{
		{
			name: "TestItemIterator_Peek 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 1,
			},
			args: args{
				num: 1,
			},
			want: []Item{
				{
					Typ:    0,
					Val:    "test",
					line:   0,
					column: 0,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			got, err := p.Peek(tt.args.num)
			if (err != nil) != tt.wantErr {
				t.Errorf("ItemIterator.Peek() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ItemIterator.Peek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItemIterator_PeekFalse(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	type args struct {
		num int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []Item
		wantErr bool
	}{
		{
			name: "TestItemIterator_Peek 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 2,
			},
			args: args{
				num: 2,
			},
			want: []Item{
				{
					Typ:    0,
					Val:    "test",
					line:   0,
					column: 0,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			got, err := p.Peek(tt.args.num)
			if (err != nil) != tt.wantErr {
				t.Errorf("ItemIterator.Peek() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if reflect.DeepEqual(got, tt.want) {
				t.Errorf("ItemIterator.Peek() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestItemIterator_PeekOne(t *testing.T) {
	type fields struct {
		l   *Lexer
		idx int
	}
	tests := []struct {
		name   string
		fields fields
		want   Item
		want1  bool
	}{
		{
			name: "TestItemIterator_PeekOne 1",
			fields: fields{
				l: &Lexer{
					Input:      "test",
					Start:      0,
					Pos:        0,
					Width:      0,
					widthStack: []*RuneWidth{},
					items: []Item{
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
						{
							Typ:    0,
							Val:    "test",
							line:   0,
							column: 0,
						},
					},
					Depth:      0,
					BlockDepth: 0,
					ArgDepth:   0,
					Mode: func(*Lexer) StateFn {
						return nil
					},
					Line:   0,
					Column: 0,
				},
				idx: 1,
			},
			want: Item{
				Typ:    0,
				Val:    "test",
				line:   0,
				column: 0,
			},
			want1: true,
		},
		// can't test false case
		// {
		// 	name: "TestItemIterator_PeekOne 2",
		// 	fields: fields{
		// 		l: &Lexer{
		// 			Input:      "test2",
		// 			Start:      0,
		// 			Pos:        0,
		// 			Width:      0,
		// 			widthStack: []*RuneWidth{},
		// 			items: []Item{
		// 				{
		// 					Typ:    0,
		// 					Val:    "test2",
		// 					line:   0,
		// 					column: 0,
		// 				},
		// 				{
		// 					Typ:    0,
		// 					Val:    "test2",
		// 					line:   0,
		// 					column: 0,
		// 				},
		// 			},
		// 			Depth:      0,
		// 			BlockDepth: 0,
		// 			ArgDepth:   0,
		// 			Mode: func(*Lexer) StateFn {
		// 				return nil
		// 			},
		// 			Line:   0,
		// 			Column: 0,
		// 		},
		// 		idx: 1,
		// 	},
		// 	want: Item{
		// 		Typ:    0,
		// 		Val:    "test2",
		// 		line:   0,
		// 		column: 0,
		// 	},
		// 	want1: false,
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ItemIterator{
				l:   tt.fields.l,
				idx: tt.fields.idx,
			}
			got, got1 := p.PeekOne()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ItemIterator.PeekOne() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ItemIterator.PeekOne() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestLexer_Reset(t *testing.T) {
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
		input string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_Reset 1",
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
				Mode: func(*Lexer) StateFn {
					return nil
				},
				Line:   0,
				Column: 0,
			},
			args: args{
				input: "reset",
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
			l.Reset(tt.args.input)
		})
	}
}

func TestLexer_ValidateResult(t *testing.T) {
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
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "TestLexer_ValidateResult 1",
			fields: fields{
				Input:      "",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Mode: func(*Lexer) StateFn {
					return nil
				},
				Line:   0,
				Column: 0,
			},
			wantErr: false,
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
			if err := l.ValidateResult(); (err != nil) != tt.wantErr {
				t.Errorf("Lexer.ValidateResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLexer_ValidateResultNext(t *testing.T) {
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
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "TestLexer_ValidateResult 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    5,
						Val:    "test 1",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Mode: func(*Lexer) StateFn {
					return nil
				},
				Line:   0,
				Column: 0,
			},
			wantErr: false,
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
			if err := l.ValidateResult(); (err != nil) != tt.wantErr {
				t.Errorf("Lexer.ValidateResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLexer_ValidateResultNextItemError(t *testing.T) {
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
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "TestLexer_ValidateResult 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    1,
						Val:    "test 1",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Mode: func(*Lexer) StateFn {
					return nil
				},
				Line:   0,
				Column: 0,
			},
			wantErr: false,
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
			if err := l.ValidateResult(); (err != nil) == tt.wantErr {
				t.Errorf("Lexer.ValidateResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// to do: need to understand how it works completely for to do false cases *****
func TestLexer_Run(t *testing.T) {
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
		f StateFn
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *Lexer
	}{
		{
			name:   "TestLexer_Run 1",
			fields: fields{},
			args: args{
				f: func(*Lexer) StateFn {
					return nil
				},
			},
			want: &Lexer{},
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
			if got := l.Run(tt.args.f); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Lexer.Run() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TODO: check how works errof
func TestLexer_Errorf(t *testing.T) {
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
		format string
		args   []interface{}
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   StateFn
	}{
		{
			name: "TestLexer_Errorf 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    1,
						Val:    "",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{
				format: "",
				args:   []interface{}{},
			},
			want: func(l *Lexer) StateFn {
				return nil
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
			if got := l.Errorf(tt.args.format, tt.args.args...); reflect.DeepEqual(got, tt.want) {
				t.Errorf("Lexer.Errorf() = %v, want %v", got, tt.want)
			}
		})
	}
}

// need to understand this func*****
func TestLexer_Emit(t *testing.T) {
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
		t ItemType
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_Emit 1",
			fields: fields{
				Input:      "test",
				Start:      1,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    0,
						Val:    "test",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{
				t: 5,
			},
		},
		{
			name:   "TestLexer_Emit 2",
			fields: fields{},
			args:   args{t: 5},
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
			l.Emit(tt.args.t)
		})
	}
}

// need to understand this func*****
func TestLexer_pushWidth(t *testing.T) {
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
		width int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_pushWidth 1",
			fields: fields{
				Input: "testing",
				Start: 0,
				Pos:   0,
				Width: 0,
				widthStack: []*RuneWidth{
					{width: 0, count: 0},
				},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{width: 6},
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
			l.pushWidth(tt.args.width)
		})
	}
}

// to do: more tests with other positions*****
func TestLexer_Next(t *testing.T) {
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
		name       string
		fields     fields
		wantResult rune
	}{
		{
			name: "TestLexer_Next 1",
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
			wantResult: 't',
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
			if gotResult := l.Next(); gotResult != tt.wantResult {
				t.Errorf("Lexer.Next() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

// need understand how to fill 'widthStack' to activate Backup() func [ok]
// what is happen if fields.Pos be? (chack this) ****
func TestLexer_Backup(t *testing.T) {
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
			name: "TestLexer_Backup 1",
			fields: fields{
				Input: "Test",
				Start: 0,
				Pos:   3,
				Width: 0,
				widthStack: []*RuneWidth{
					{width: 1, count: 1},
				},
				items:      []Item{},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
		},
		// Test false case (fields must be empty)
		// {
		// 	name:   "TestLexer_Backup 2",
		// 	fields: fields{},
		// },
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
			l.Backup()
		})
	}
}

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

func TestLexer_PeekTwoEOF(t *testing.T) {
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
				Input:      "",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    -1,
						Val:    "",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			want: []rune{'e', 'e'},
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
			if got := l.PeekTwo(); reflect.DeepEqual(got, tt.want) {
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
				Input:      "This a simple test",
				Start:      0,
				Pos:        2,
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
				Pos:        1,
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

func TestLexer_moveStartToPosEOL(t *testing.T) {
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
				Input:      "t\u000A",
				Start:      0,
				Pos:        2,
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
			name: "TestLexer_moveStartToPos 1",
			fields: fields{
				Input:      "t\u000D",
				Start:      0,
				Pos:        2,
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
				Input:      "Test",
				Start:      0,
				Pos:        1,
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

func TestLexer_AcceptRun(t *testing.T) {
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
		c CheckRune
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantLastr  rune
		wantValidr bool
	}{
		{
			name: "TestLexer_AcceptRun 1",
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
			args: args{
				c: func(r rune) bool {
					if r == 't' {
						return true
					}
					return false
				},
			},
			wantLastr:  't',
			wantValidr: true,
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
			gotLastr, gotValidr := l.AcceptRun(tt.args.c)
			if gotLastr != tt.wantLastr {
				t.Errorf("Lexer.AcceptRun() gotLastr = %v, want %v", gotLastr, tt.wantLastr)
			}
			if gotValidr != tt.wantValidr {
				t.Errorf("Lexer.AcceptRun() gotValidr = %v, want %v", gotValidr, tt.wantValidr)
			}
		})
	}
}

func TestLexer_AcceptRunRec(t *testing.T) {
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
		c CheckRuneRec
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_AcceptRunRec 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        1,
				Width:      4,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{
				c: func(r rune, l *Lexer) bool {
					if r == 'e' {
						return true
					}
					return false
				},
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
			l.AcceptRunRec(tt.args.c)
		})
	}
}

func TestLexer_AcceptUntil(t *testing.T) {
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
		c CheckRune
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_AcceptUntil 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        1,
				Width:      4,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{
				c: func(r rune) bool {
					if r == 'e' {
						return true
					}
					return false
				},
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
			l.AcceptUntil(tt.args.c)
		})
	}
}

func TestLexer_AcceptRunTimes(t *testing.T) {
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
		c     CheckRune
		times int
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   int
	}{
		{
			name: "TestLexer_AcceptRunTimes 1",
			fields: fields{
				Input:      "test",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Mode: func(*Lexer) StateFn {
					return nil
				},
				Line:   0,
				Column: 0,
			},
			args: args{
				c: func(r rune) bool {
					if r == 'e' {
						return true
					}
					return false
				},
				times: 2,
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
			if got := l.AcceptRunTimes(tt.args.c, tt.args.times); got != tt.want {
				t.Errorf("Lexer.AcceptRunTimes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLexer_IgnoreRun(t *testing.T) {
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
		c CheckRune
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "TestLexer_IgnoreRun 1",
			fields: fields{
				Input:      "Test string",
				Start:      0,
				Pos:        0,
				Width:      0,
				widthStack: []*RuneWidth{},
				items: []Item{
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			args: args{
				c: func(r rune) bool {
					if r == 'e' {
						return true
					}
					return false
				},
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
			l.IgnoreRun(tt.args.c)
		})
	}
}

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

func TestLexer_LexQuotedString(t *testing.T) {
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
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "TestLexer_LexQuotedString 1",
			fields: fields{
				Input: "string",
				Start: 0,
				Pos:   4,
				Width: 4,
				widthStack: []*RuneWidth{
					{
						width: 4,
						count: 0,
					},
				},
				items: []Item{
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "s",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			wantErr: true,
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
			if err := l.LexQuotedString(); (err != nil) != tt.wantErr {
				t.Errorf("Lexer.LexQuotedString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLexer_LexQuotedStringQuote(t *testing.T) {
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
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "TestLexer_LexQuotedString 1",
			fields: fields{
				Input: "test",
				Start: 0,
				Pos:   4,
				Width: 4,
				widthStack: []*RuneWidth{
					{
						width: 4,
						count: 0,
					},
				},
				items: []Item{
					{
						Typ:    5,
						Val:    "'",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "e",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "s",
						line:   0,
						column: 0,
					},
					{
						Typ:    5,
						Val:    "t",
						line:   0,
						column: 0,
					},
				},
				Depth:      0,
				BlockDepth: 0,
				ArgDepth:   0,
				Line:       0,
				Column:     0,
			},
			wantErr: true,
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
			if err := l.LexQuotedString(); (err != nil) != tt.wantErr {
				t.Errorf("Lexer.LexQuotedString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
