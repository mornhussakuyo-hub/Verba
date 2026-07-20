package ast

type Position struct {
	File   string
	Offset int
	Line   int
	Column int
}

type File struct {
	Path   string
	Module string
	Uses   []Use
	Decls  []Decl
}

type Use struct {
	Parts []string
	Pos   Position
}

type Decl interface {
	DeclName() string
	DeclPos() Position
}

type Field struct {
	Name string
	Type Type
	Pos  Position
}

type Record struct {
	Name   string
	Fields []Field
	Pos    Position
}

func (d *Record) DeclName() string  { return d.Name }
func (d *Record) DeclPos() Position { return d.Pos }

type Enum struct {
	Name  string
	Cases []Field
	Pos   Position
}

func (d *Enum) DeclName() string  { return d.Name }
func (d *Enum) DeclPos() Position { return d.Pos }

type Function struct {
	Name   string
	Inputs []Field
	Output *Type
	Body   []Stmt
	Pos    Position
}

func (d *Function) DeclName() string  { return d.Name }
func (d *Function) DeclPos() Position { return d.Pos }

type Route struct {
	Name   string
	Method string
	Path   string
	Output *Type
	Body   []Stmt
	Pos    Position
}

func (d *Route) DeclName() string  { return d.Name }
func (d *Route) DeclPos() Position { return d.Pos }

type Embed struct {
	Kind       string
	Name       string
	Terminator string
	Raw        string
	RawStart   int
	RawEnd     int
	SQL        *SQLQuery
	Pos        Position
}

func (d *Embed) DeclName() string  { return d.Name }
func (d *Embed) DeclPos() Position { return d.Pos }

type SQLColumn struct {
	Name string
	Type Type
}

type SQLParameter struct {
	Name string
	Type Type
}

type SQLQuery struct {
	Statement  string
	Text       string
	Parameters []SQLParameter
	Columns    []SQLColumn
	RowType    Type
}

type Type struct {
	Name string
	Args []Type
}

func (t Type) String() string {
	result := t.Name
	for _, arg := range t.Args {
		result += " " + arg.String()
	}
	return result
}

type ExprKind int

const (
	ExprInvalid ExprKind = iota
	ExprAtom
	ExprText
	ExprCall
	ExprGet
	ExprRelation
)

type Expr struct {
	Kind           ExprKind
	Value          string
	LiteralType    string
	ResolvedType   Type
	CallResultType Type
	Args           []Expr
	NamedArgs      []NamedArg
	Try            bool
	Not            bool
	Pos            Position
}

type NamedArg struct {
	Name  string
	Value Expr
	Pos   Position
}

type Stmt interface {
	StmtPos() Position
}

type LetStmt struct {
	Name    string
	Mutable bool
	Value   Expr
	Pos     Position
}

func (s *LetStmt) StmtPos() Position { return s.Pos }

type SetStmt struct {
	Path  []string
	Value Expr
	Pos   Position
}

func (s *SetStmt) StmtPos() Position { return s.Pos }

type ExprStmt struct {
	Value Expr
	Pos   Position
}

func (s *ExprStmt) StmtPos() Position { return s.Pos }

type ReturnStmt struct {
	Value *Expr
	Pos   Position
}

func (s *ReturnStmt) StmtPos() Position { return s.Pos }

type RespondStmt struct {
	Format string
	Status int
	Value  *Expr
	Pos    Position
}

func (s *RespondStmt) StmtPos() Position { return s.Pos }

type IfStmt struct {
	Condition Expr
	Then      []Stmt
	Else      []Stmt
	Pos       Position
}

func (s *IfStmt) StmtPos() Position { return s.Pos }

type ForStmt struct {
	Name     string
	Iterable Expr
	Body     []Stmt
	Pos      Position
}

func (s *ForStmt) StmtPos() Position { return s.Pos }

type WhileStmt struct {
	Condition Expr
	Body      []Stmt
	Pos       Position
}

func (s *WhileStmt) StmtPos() Position { return s.Pos }

type MatchCase struct {
	Pattern Expr
	Body    []Stmt
	Pos     Position
}

type MatchStmt struct {
	Value Expr
	Cases []MatchCase
	Else  []Stmt
	Pos   Position
}

func (s *MatchStmt) StmtPos() Position { return s.Pos }

type TransactionStmt struct {
	Resource string
	Body     []Stmt
	Pos      Position
}

func (s *TransactionStmt) StmtPos() Position { return s.Pos }
