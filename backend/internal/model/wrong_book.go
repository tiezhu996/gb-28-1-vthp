package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WrongBook 错题本实体，集合 wrong_books。
// 状态枚举：active / resolved。
// 交卷时自动收录客观错题：首次收录保留试卷答案解析；再次答错改存最近一次
// 试卷/作答/交卷时间，WrongCount 加一、状态回到 active，Note 与 CreatedAt 保留。
type WrongBook struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	StudentID       primitive.ObjectID `bson:"student_id" json:"student_id"`
	QuestionID      primitive.ObjectID `bson:"question_id" json:"question_id"`
	ExamID          primitive.ObjectID `bson:"exam_id" json:"exam_id"`        // 最近一次出错的试卷
	ExamRecordID    primitive.ObjectID `bson:"exam_record_id" json:"exam_record_id"` // 最近一次出错的答卷
	Subject         string             `bson:"subject" json:"subject"`
	KnowledgePoints []string           `bson:"knowledge_points" json:"knowledge_points"`
	QuestionContent string             `bson:"question_content" json:"question_content"`
	MyAnswer        string             `bson:"my_answer" json:"my_answer"` // 最近一次答错的作答
	CorrectAnswer   string             `bson:"correct_answer" json:"correct_answer"`
	Analysis        string             `bson:"analysis" json:"analysis"`   // 首次收录时的试卷答案解析
	Note            string             `bson:"note" json:"note"`           // 学生备注（再次答错时保留）
	Status          string             `bson:"status" json:"status"`
	WrongCount      int                `bson:"wrong_count" json:"wrong_count"`   // 累计答错次数
	LastWrongAt     time.Time          `bson:"last_wrong_at" json:"last_wrong_at"` // 最近出错时间（最近一次交卷时间）
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`     // 首次收录时间
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}
