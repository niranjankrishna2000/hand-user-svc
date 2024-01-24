package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
	"user_svc/pkg/db"
	"user_svc/pkg/models"
	pb "user_svc/pkg/pb"

	"github.com/razorpay/razorpay-go"
	"gorm.io/gorm"
)

type Server struct {
	H db.Handler
	pb.UnimplementedUserServiceServer
}

func (s *Server) CreatePost(ctx context.Context, req *pb.CreatePostRequest) (*pb.CreatePostResponse, error) {

	log.Println("Post creation started")
	var PostID int32
	layout := "2006-01-02 15:04:05"
	timestamp, err := time.Parse(layout, req.Date)
	if err != nil {
		fmt.Println("Error parsing string:", err)
		return &pb.CreatePostResponse{
			Status:   http.StatusBadRequest,
			Response: "Error Parsing time string",
			Post:     &pb.Post{},
		}, err
	}
	query := `
    INSERT INTO posts (text, place,image, date, amount,user_id,account_no,address)
    VALUES (?, ?, ?, ?, ?,?,?,?) RETURNING id
	`
	s.H.DB.Raw(query, req.Text, req.Place, req.Image, timestamp, req.Amount, req.Userid, req.Accno, req.Address).Scan(&PostID)
	var postdetails *pb.Post

	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", PostID).Scan(&postdetails).Error; err != nil {
		return &pb.CreatePostResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB",
			Post:     &pb.Post{},
		}, err
	}
	return &pb.CreatePostResponse{
		Status:   http.StatusCreated,
		Response: "",
		Post:     postdetails,
	}, nil
}

func (s *Server) UserFeeds(ctx context.Context, req *pb.UserFeedsRequest) (*pb.UserFeedsResponse, error) {

	log.Println("Feeds collection started")
	log.Println("Data collected", req)
	var page, limit int64
	page, limit = int64(req.Page), int64(req.Limit)
	// pagination purpose -
	if req.Page == 0 {
		page = 1
	}
	if req.Limit == 0 {
		limit = 10
	}
	offset := (page - 1) * limit
	var postdetails []*pb.Post

	sqlQuery := "SELECT * FROM posts WHERE status = 'approved'"
	if req.Searchkey != "" {
		sqlQuery += " AND (text ILIKE '%" + req.Searchkey + "%' OR place ILIKE '%" + req.Searchkey + "%')"
	}
	sqlQuery += " ORDER BY date DESC, amount DESC LIMIT ? OFFSET ?"

	if err := s.H.DB.Raw(sqlQuery, limit, offset).Scan(&postdetails).Error; err != nil {
		return &pb.UserFeedsResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB",
			Posts:    []*pb.Post{},
		}, err
	}
	log.Println("feeds:", postdetails)
	return &pb.UserFeedsResponse{
		Status:   http.StatusOK,
		Response: "",
		Posts:    postdetails,
	}, nil

}

func (s *Server) UserPostDetails(ctx context.Context, req *pb.UserPostDetailsRequest) (*pb.UserPostDetailsResponse, error) {

	log.Println("Post detailes started")

	var post pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts WHERE id=? AND (status = 'approved' OR status ='expired')", req.PostID).Scan(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.UserPostDetailsResponse{
				Status:   http.StatusNotFound,
				Response: "Post Not Found",
				Post:     &pb.PostDetails{},
			}, nil
		}
		// return &pb.UserPostDetailsResponse{
		// 	Status:   http.StatusInternalServerError,
		// 	Response: "Failed to fetch post details",
		// 	Post:     &pb.PostDetails{},
		// }, err
	}
	Comments, err := s.GetComments(int(req.PostID))
	if err != nil {
		return &pb.UserPostDetailsResponse{Status: http.StatusBadGateway, Response: "Could not get comments from db", Post: &pb.PostDetails{Post: &post}}, err
	}
	postdetails := &pb.PostDetails{
		Post:     &post,
		Comments: Comments,
	}
	err = s.H.DB.Exec("UPDATE posts set views = views+1 where id = ?", req.PostID).Error
	if err != nil {
		fmt.Println(err)
		return &pb.UserPostDetailsResponse{Status: http.StatusBadGateway, Response: "Could not Update views"}, err
	}
	return &pb.UserPostDetailsResponse{
		Status:   http.StatusOK,
		Response: "Successfully got the post",
		Post:     postdetails,
	}, nil

}

func (s *Server) Donate(ctx context.Context, req *pb.DonateRequest) (*pb.DonateResponse, error) {

	log.Println("Donation started")
	var postdetails *pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", req.Postid).Scan(&postdetails).Error; err != nil {
		return &pb.DonateResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
		}, err
	}
	//create new data and return id
	var payID int32
	query := `
    INSERT INTO payments (user_id, post_id,amount, date)
    VALUES (?, ?, ?, ?) RETURNING id
	`
	s.H.DB.Raw(query, req.Userid, req.Postid, req.Amount, time.Now()).Scan(&payID)

	link := fmt.Sprintf("http://localhost:1111/user/post/donate/razorpay?payid=%d", payID)

	return &pb.DonateResponse{
		Status:   http.StatusOK,
		Response: "",
		Link:     link,
		Post:     postdetails,
	}, nil

}

func (s *Server) MakePaymentRazorPay(ctx context.Context, req *pb.MakePaymentRazorPayRequest) (*pb.MakePaymentRazorPayResponse, error) {
	var postDetails pb.MakePaymentRazorPayResponse
	var paymentdetail models.Payment
	if err := s.H.DB.Raw("SELECT * FROM payments WHERE id=? AND status = 'pending'", req.Payid).Scan(&paymentdetail).Error; err != nil {
		return &pb.MakePaymentRazorPayResponse{
			Status:   http.StatusBadRequest,
			Response: "Payment Not Available",
		}, err
	}
	log.Println("collected data:", paymentdetail)
	newid := paymentdetail.PostID
	postDetails.PaymentID = int32(newid)

	postDetails.UserID = int32(paymentdetail.UserID)

	//note
	//postDetails.Username = paymentdetail

	postDetails.FinalPrice = int64(paymentdetail.Amount)

	err := s.H.DB.Exec("UPDATE posts set collected = collected + ? where id = ?", paymentdetail.Amount, paymentdetail.PostID).Error
	if err != nil {
		fmt.Println(err)
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: err.Error()}, err
	}
	client := razorpay.NewClient("rzp_test_zzmWMLGS9uRsb7", "WzzMnKdMFWY91e2DGBiZMFN8")

	data := map[string]interface{}{
		"amount":   int(postDetails.FinalPrice) * 100,
		"currency": "INR",
		"receipt":  "some_receipt_id",
	}
	log.Println("razorpay::91 ", data)

	body, err := client.Order.Create(data, nil)
	if err != nil {
		fmt.Println(err)
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: err.Error()}, err
	}

	razorPayOrderID := body["id"].(string)

	postDetails.RazorId = razorPayOrderID
	//fmt.Println("razorpay::100", postDetails)postIDstr
	err = s.H.DB.Exec("UPDATE payments set status = 'completed' , payment_id = ? where id = ?", razorPayOrderID, paymentdetail.Id).Error
	if err != nil {
		fmt.Println(err)
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: err.Error()}, err
	}
	//notify
	// query := `
	// INSERT INTO notifications (user_id, post_id,time, text, type)
	// VALUES (?, ?, ?, ?, ?,?,?,?) RETURNING id
	// `
	// s.H.DB.Raw(query, req.Text, req.Place, req.Image, timestamp, req.Amount, req.Userid, req.Accno, req.Address).Scan(&PostID)
	// var postdetails *pb.Post

	return &postDetails, nil
}

func (s *Server) GenerateInvoice(ctx context.Context, req *pb.GenerateInvoiceRequest) (*pb.GenerateInvoiceResponse, error) {

	var paymentdetail models.Payment
	var address string
	if err := s.H.DB.Raw("SELECT * FROM payments WHERE payment_id=? AND status = 'completed'", req.InvoiceId).Scan(&paymentdetail).Error; err != nil {
		return &pb.GenerateInvoiceResponse{
			Status:   http.StatusBadRequest,
			Response: "Payment Data Not Available",
		}, err
	}
	if err := s.H.DB.Raw("SELECT CONCAT(account_no, ' ', address) FROM posts WHERE id=?", paymentdetail.PostID).Scan(&address).Error; err != nil {
		return &pb.GenerateInvoiceResponse{
			Status:   http.StatusBadRequest,
			Response: "Payment Data Not Available",
		}, err
	}

	log.Println("collected data:", paymentdetail, address)

	return &pb.GenerateInvoiceResponse{
		Status:     http.StatusOK,
		Response:   "",
		UserID:     int32(paymentdetail.UserID),
		Address:    address,
		FinalPrice: int64(paymentdetail.Amount),
	}, nil

}

// report
func (s *Server) ReportPost(ctx context.Context, req *pb.ReportPostRequest) (*pb.ReportPostResponse, error) {
	log.Println("ReportPost started")
	log.Println("Collected Data: ", req)

	if req.Postid < 1 {
		log.Println("Invalid Post ID")
		return &pb.ReportPostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid Post ID",
		}, errors.New("invalid Post ID")
	}
	if req.Userid < 1 {
		log.Println("Invalid user ID")
		return &pb.ReportPostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	if req.Text == "" {
		return &pb.ReportPostResponse{
			Status:   http.StatusBadRequest,
			Response: "no text found",
		}, errors.New("no text found")
	}
	var postId int32
	query := `
    INSERT INTO reporteds (reason,user_id,post_id,category)
    VALUES (?, ?, ?, ?) RETURNING post_id
	`
	s.H.DB.Raw(query, req.Text, req.Userid, req.Postid, "POST").Scan(&postId)
	var postdetails *pb.Post

	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", postId).Scan(&postdetails).Error; err != nil {
		return &pb.ReportPostResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB",
		}, err
	}
	return &pb.ReportPostResponse{
		Status:   http.StatusOK,
		Response: "Successfully Reported Post",
		Post:     postdetails,
	}, nil
}

func (s *Server) ReportComment(ctx context.Context, req *pb.ReportCommentRequest) (*pb.ReportCommentResponse, error) {
	log.Println("ReportComment started")
	log.Println("Collected Data: ", req)

	if req.Commentid < 1 {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid comment ID",
		}, errors.New("invalid comment ID")
	}
	if req.Userid < 1 {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	if req.Text == "" {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "no text found",
		}, errors.New("no text found")
	}
	var postId int
	query := `
    INSERT INTO reporteds (reason,user_id,comment_id,category)
    VALUES (?, ?, ?, ?)
	`
	if err := s.H.DB.Raw(query, req.Text, req.Userid, req.Commentid, "COMMENT").Error; err != nil {
		log.Printf("Failed to insert report: %v", err)
		return &pb.ReportCommentResponse{
			Status:   http.StatusInternalServerError,
			Response: "Failed to insert report",
		}, err
	}
	if err := s.H.DB.Raw("SELECT post_id FROM comments where id=?", req.Commentid).Scan(&postId).Error; err != nil {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get postid from DB",
		}, errors.New("could not get postid from DB")
	}
	var postdetails *pb.PostDetails

	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", postId).Scan(&postdetails.Post).Error; err != nil {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
		}, errors.New("could not get post from DB")
	}

	if err := s.H.DB.Raw("SELECT * FROM comments where post_id=?", postId).Scan(&postdetails.Comments).Error; err != nil {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
		}, errors.New("could not get comments from DB")
	}
	return &pb.ReportCommentResponse{
		Status:   http.StatusOK,
		Response: "Successfully Reported comment",
		Post:     postdetails,
	}, nil
}

// post
func (s *Server) EditPost(ctx context.Context, req *pb.EditPostRequest) (*pb.EditPostResponse, error) {

	log.Println("EditPost started")
	if req.Postid < 1 {
		return &pb.EditPostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid post ID",
		}, errors.New("invalid post ID")
	}
	if req.Accno != "" {
		err := s.H.DB.Exec("UPDATE posts set account_no = ? where id = ?", req.Accno, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update Account No"}, err
		}
	}
	if req.Address != "" {
		err := s.H.DB.Exec("UPDATE posts set address = ? where id = ?", req.Address, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update Address"}, err
		}
	}
	if req.Date != "" {
		layout := "2006-01-02 15:04:05"
		timestamp, err := time.Parse(layout, req.Date)
		if err != nil {
			fmt.Println("Error parsing string:", err)
			return &pb.EditPostResponse{
				Status:   http.StatusBadRequest,
				Response: "Error Parsing time string",
			}, err
		}
		err = s.H.DB.Exec("UPDATE posts set date = ? where id = ?", timestamp, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update date"}, err
		}
	}
	if req.Image != "" {
		err := s.H.DB.Exec("UPDATE posts set image = ? where id = ?", req.Image, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update Image"}, err
		}
	}
	if req.Place != "" {
		err := s.H.DB.Exec("UPDATE posts set place = ? where id = ?", req.Place, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update place"}, err
		}
	}
	if req.Text != "" {
		err := s.H.DB.Exec("UPDATE posts set text = ? where id = ?", req.Text, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update text"}, err
		}
	}
	if req.Amount > 1 {
		err := s.H.DB.Exec("UPDATE posts set amount = ? where id = ?", req.Amount, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update Amount"}, err
		}
	}
	var postdetails *pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", req.Postid).Scan(&postdetails).Error; err != nil {
		return &pb.EditPostResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
		}, errors.New("could not get post from DB")
	}
	return &pb.EditPostResponse{
		Status:   http.StatusOK,
		Response: "Successfully Updated Post",
		Post:     postdetails,
	}, nil
}

func (s *Server) LikePost(ctx context.Context, req *pb.LikePostRequest) (*pb.LikePostResponse, error) {
	log.Println("LikePost started")
	log.Println("Collected Data: ", req)

	if req.Postid < 1 {
		log.Println("Invalid Post ID")
		return &pb.LikePostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid post ID",
		}, errors.New("invalid post ID")
	}
	if req.Userid < 1 {
		log.Println("Invalid user ID")
		return &pb.LikePostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	var count int
	if err := s.H.DB.Raw(`SELECT COUNT(*)
	FROM likes
	WHERE postid = ? AND userid = ?`, req.Postid, req.Userid).Scan(&count).Error; err != nil {
		log.Println("already liked the post")
		return &pb.LikePostResponse{
			Status:   http.StatusBadRequest,
			Response: "already liked the post",
		}, err
	}
	log.Println("Count", count)
	if count == 0 {
		log.Println("Have not liked already ")

		query := `
    	INSERT INTO likes (userid, postid)
    	VALUES (?, ?);
	`
		if err := s.H.DB.Exec(query, req.Userid, req.Postid).Error; err != nil {
			log.Printf("Failed to like post: %v", err)
			return &pb.LikePostResponse{
				Status:   http.StatusInternalServerError,
				Response: "Failed to like post",
			}, err
		}
		
		log.Println("Adding into likes ")
		err := s.H.DB.Exec("UPDATE posts set likes = likes + 1 where id = ?", req.Postid).Error
		if err != nil {
			log.Println("could not like the post")
			return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "could not like the post"}, err
		}
	} else {
		err := s.H.DB.Exec("delete from likes where userid = ? and postid=?", req.Userid, req.Postid).Error
		if err != nil {
			return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "already liked!! Could not remove the like"}, err
		}
		err = s.H.DB.Exec("UPDATE posts set likes = likes - 1 where id = ?", req.Postid).Error
		if err != nil {
			return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "already liked!! Could not remove the like"}, err
		}
		Post, err := s.Getpost(int(req.Postid))
		if err != nil {
			return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "Successfully removed Like!! Could not get post from db"}, err
		}
		Comments, err := s.GetComments(int(req.Postid))
		if err != nil {
			return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "Successfully removed Like!! Could not get comments from db", Post: &pb.PostDetails{Post: Post}}, err
		}
		return &pb.LikePostResponse{
			Status:   http.StatusOK,
			Response: "successfully removed the like",
			Post:     &pb.PostDetails{Post: Post, Comments: Comments},
		}, nil
	}
	Post, err := s.Getpost(int(req.Postid))
	if err != nil {
		return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "Successfully Liked!! Could not get post from db"}, err
	}
	Comments, err := s.GetComments(int(req.Postid))
	if err != nil {
		return &pb.LikePostResponse{Status: http.StatusBadGateway, Response: "Successfully Liked!! Could not get comments from db", Post: &pb.PostDetails{Post: Post}}, err
	}

	//notify
	err = s.Notify(int(req.Userid), int(Post.UserId), int(Post.Id), "like")
	if err != nil {
		return &pb.LikePostResponse{
			Status:   http.StatusOK,
			Response: "successfully Liked the post, but could not notify",
			Post:     &pb.PostDetails{Post: Post, Comments: Comments},
		}, nil
	}
	log.Println("Finished Like post")
	return &pb.LikePostResponse{
		Status:   http.StatusOK,
		Response: "successfully Liked the post",
		Post:     &pb.PostDetails{Post: Post, Comments: Comments},
	}, nil
}

func (s *Server) CommentPost(ctx context.Context, req *pb.CommentPostRequest) (*pb.CommentPostResponse, error) {
	log.Println("CommentPost started")
	log.Println("Collected Data: ", req)

	if req.Postid < 1 {
		return &pb.CommentPostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid post ID",
		}, errors.New("invalid post ID")
	}
	if req.Userid < 1 {
		return &pb.CommentPostResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	if req.Comment == "" {
		return &pb.CommentPostResponse{
			Status:   http.StatusBadRequest,
			Response: "text unavailable",
		}, errors.New("text unavailable")
	}
	log.Println("Validation Complete")
	query := `
    	INSERT INTO comments (user_id, post_id,comment,time)
    	VALUES (?, ?,?,?);
	`
	currentTime := time.Now()
	if err := s.H.DB.Exec(query, req.Userid, req.Postid, req.Comment, currentTime).Error; err != nil {
		log.Printf("Failed to insert comment: %v", err)
		return &pb.CommentPostResponse{
			Status:   http.StatusInternalServerError,
			Response: "Failed to insert comment",
		}, err
	}
	Post, err := s.Getpost(int(req.Postid))
	if err != nil {
		return &pb.CommentPostResponse{Status: http.StatusBadGateway, Response: "Successfully commented!! Could not get post from db"}, err
	}
	Comments, err := s.GetComments(int(req.Postid))
	if err != nil {
		return &pb.CommentPostResponse{Status: http.StatusBadGateway, Response: "Successfully commented!! Could not get comments from db", Post: &pb.PostDetails{Post: Post}}, err
	}

	//notify
	err = s.Notify(int(req.Userid), int(Post.UserId), int(Post.Id), "comment")
	if err != nil {
		return &pb.CommentPostResponse{
			Status:   http.StatusOK,
			Response: "successfully commented the post, but could not notify",
			Post:     &pb.PostDetails{Post: Post, Comments: Comments},
		}, nil
	}
	return &pb.CommentPostResponse{
		Status:   http.StatusOK,
		Response: "successfully commented the post",
		Post:     &pb.PostDetails{Post: Post, Comments: Comments},
	}, nil
}

func (s *Server) DeleteComment(ctx context.Context, req *pb.DeleteCommentRequest) (*pb.DeleteCommentResponse, error) {
	log.Println("DeleteComment started")
	log.Println("Collected Data: ", req)

	if req.Commentid < 1 {
		return &pb.DeleteCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid comment ID",
		}, errors.New("invalid comment ID")
	}
	if req.Userid < 1 {
		return &pb.DeleteCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	var count int
	var postId int
	if err := s.H.DB.Raw(`SELECT COUNT(*)
	FROM comments
	WHERE id = ? AND user_id = ?`, req.Commentid, req.Userid).Scan(&count).Error; err != nil {
		return &pb.DeleteCommentResponse{
			Status:   http.StatusNotFound,
			Response: "no such comment or Unauthorized",
		}, err
	}
	if count < 1 {
		return &pb.DeleteCommentResponse{
			Status:   http.StatusNotFound,
			Response: "no such comment or Unauthorized",
		}, errors.New("no such comment found or Unauthorized")
	} else {
		if err := s.H.DB.Raw(`SELECT post_id
		FROM comments
		WHERE id = ? AND user_id = ?`, req.Commentid, req.Userid).Scan(&postId).Error; err != nil {
			return &pb.DeleteCommentResponse{
				Status:   http.StatusNotFound,
				Response: "could not get the post details",
			}, err
		}
		err := s.H.DB.Exec("delete from comments where id=?", req.Commentid).Error
		if err != nil {
			return &pb.DeleteCommentResponse{Status: http.StatusBadGateway, Response: "Could not remove the comment"}, err
		}
	}
	Post, err := s.Getpost(int(postId))
	if err != nil {
		return &pb.DeleteCommentResponse{Status: http.StatusBadGateway, Response: "Successfully commented!! Could not get post from db"}, err
	}
	Comments, err := s.GetComments(int(postId))
	if err != nil {
		return &pb.DeleteCommentResponse{Status: http.StatusBadGateway, Response: "Successfully commented!! Could not get comments from db", Post: &pb.PostDetails{Post: Post}}, err
	}
	return &pb.DeleteCommentResponse{
		Status:   http.StatusOK,
		Response: "Successfully removed the comment",
		Post:     &pb.PostDetails{Post: Post, Comments: Comments},
	}, nil
}

// donation
func (s *Server) DonationHistory(ctx context.Context, req *pb.DonationHistoryRequest) (*pb.DonationHistoryResponse, error) {
	log.Println("Donation History started")
	var page, limit int64
	page, limit = int64(req.Page), int64(req.Limit)
	// pagination purpose -
	if req.Page <= 0 {
		page = 1
	}
	if req.Limit <= 0 {
		limit = 10
	}
	offset := (page - 1) * limit

	// var donations []*pb.Donation
	var donationlist []*models.Payment
	sqlQuery := "SELECT * FROM payments WHERE status = 'completed' and user_id=?"
	if req.Searchkey != "" {
		sqlQuery += " AND (text ILIKE '%" + req.Searchkey + "%' OR place ILIKE '%" + req.Searchkey + "%')"
	}
	sqlQuery += " ORDER BY date DESC, amount DESC LIMIT ? OFFSET ?"

	if err := s.H.DB.Raw(sqlQuery, req.Userid, limit, offset).Scan(&donationlist).Error; err != nil {
		return &pb.DonationHistoryResponse{
			Status:    http.StatusBadRequest,
			Response:  "couldn't get posts from DB",
			Donations: []*pb.Donation{},
		}, err
	}
	//iterate and copy
	return &pb.DonationHistoryResponse{
		Status:    http.StatusOK,
		Response:  "successfully retrieved Donation history",
		Donations: []*pb.Donation{},
	}, nil
}

func (s *Server) ClearHistory(ctx context.Context, req *pb.ClearHistoryRequest) (*pb.ClearHistoryResponse, error) {
	log.Println("ClearHistory started")
	log.Println("Collected Data: ", req)

	if req.Userid < 1 {
		return &pb.ClearHistoryResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	err := s.H.DB.Exec("delete from payments where user_id=?", req.Userid).Error
	if err != nil {
		return &pb.ClearHistoryResponse{Status: http.StatusBadGateway, Response: "Could not clear the history"}, err
	}
	return &pb.ClearHistoryResponse{
		Status:   http.StatusOK,
		Response: "Successfully Cleared history",
	}, nil
}

// notification
func (s *Server) Notifications(ctx context.Context, req *pb.NotificationRequest) (*pb.NotificationResponse, error) {
	log.Println("Notifications started")
	log.Println("Collected Data: ", req)
	if req.Userid < 1 {
		return &pb.NotificationResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	var page, limit int64
	page, limit = int64(req.Page), int64(req.Limit)
	// pagination purpose -
	if req.Page <= 0 {
		page = 1
	}
	if req.Limit <= 0 {
		limit = 10
	}
	offset := (page - 1) * limit
	var notifications []*pb.Notification
	sqlQuery := "SELECT * FROM notifications WHERE  user_id=? ORDER BY time DESC LIMIT ? OFFSET ?"

	if err := s.H.DB.Raw(sqlQuery, req.Userid, limit, offset).Scan(&notifications).Error; err != nil {
		return &pb.NotificationResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get notifications from DB",
		}, err
	}

	return &pb.NotificationResponse{
		Status:        http.StatusOK,
		Response:      "Successfully retrieved notifications",
		Notifications: notifications,
	}, nil
}

// note: 3 type notifications
// **donation
// **like
// **comment
// **request
func (s *Server) NotificationDetail(ctx context.Context, req *pb.NotificationDetailsRequest) (*pb.NotificationDetailsResponse, error) {
	log.Println("NotificationDetail started")
	log.Println("Collected Data: ", req)
	if req.Userid < 1 {
		return &pb.NotificationDetailsResponse{
			Status:       http.StatusBadRequest,
			Response:     "Invalid user ID",
			Notification: &pb.Notification{},
		}, errors.New("invalid user ID")
	}
	if req.Notificationid < 1 {
		return &pb.NotificationDetailsResponse{
			Status:       http.StatusBadRequest,
			Response:     "Invalid notification ID",
			Notification: &pb.Notification{},
		}, errors.New("invalid notification ID")
	}
	var notification *pb.Notification
	sqlQuery := "SELECT * FROM notifications WHERE  user_id=? and id=?'"
	if err := s.H.DB.Raw(sqlQuery, req.Userid, req.Notificationid).Scan(&notification).Error; err != nil {
		return &pb.NotificationDetailsResponse{
			Status:       http.StatusBadRequest,
			Response:     "couldn't get notification from DB",
			Notification: &pb.Notification{},
		}, err
	}
	var postId int
	sqlQuery = "SELECT post_id FROM notifications WHERE  user_id=? and id=?'"
	if err := s.H.DB.Raw(sqlQuery, req.Userid, req.Notificationid).Scan(&postId).Error; err != nil {
		return &pb.NotificationDetailsResponse{
			Status:       http.StatusBadRequest,
			Response:     "couldn't get post from DB",
			Notification: notification,
		}, err
	}
	Post, err := s.Getpost(postId)
	if err != nil {
		return &pb.NotificationDetailsResponse{Status: http.StatusBadGateway, Response: "Successfully got notification details!!But Could not get post from db", Notification: notification}, err
	}
	notification.Post.Post = Post
	Comments, err := s.GetComments(int(postId))
	if err != nil {
		return &pb.NotificationDetailsResponse{Status: http.StatusBadGateway, Response: "Successfully got notification details!! Could not get comments from db", Notification: notification}, err
	}
	notification.Post.Comments = Comments
	return &pb.NotificationDetailsResponse{
		Status:       http.StatusOK,
		Response:     "Successfully got the notification details",
		Notification: notification,
	}, nil
}

func (s *Server) DeleteNotification(ctx context.Context, req *pb.DeleteNotificationRequest) (*pb.DeleteNotificationResponse, error) {
	log.Println("DeleteNotification started")
	log.Println("Collected Data: ", req)
	if req.Userid < 1 {
		return &pb.DeleteNotificationResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	if req.Notificationid < 1 {
		return &pb.DeleteNotificationResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid notification ID",
		}, errors.New("invalid notification ID")
	}
	err := s.H.DB.Exec("delete from notifications where user_id=? and id=?", req.Userid, req.Notificationid).Error
	if err != nil {
		return &pb.DeleteNotificationResponse{Status: http.StatusBadGateway, Response: "Could not delete the notification"}, err
	}
	return &pb.DeleteNotificationResponse{
		Status:   http.StatusOK,
		Response: "Successfully deleted the notification",
	}, nil
}
func (s *Server) ClearNotification(ctx context.Context, req *pb.ClearNotificationRequest) (*pb.ClearNotificationResponse, error) {
	log.Println("ClearNotification started")
	log.Println("Collected Data: ", req)
	if req.Userid < 1 {
		return &pb.ClearNotificationResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid user ID",
		}, errors.New("invalid user ID")
	}
	err := s.H.DB.Exec("delete from notifications where user_id=?", req.Userid).Error
	if err != nil {
		return &pb.ClearNotificationResponse{Status: http.StatusBadGateway, Response: "Could not clear the notification"}, err
	}
	return &pb.ClearNotificationResponse{
		Status:   http.StatusOK,
		Response: "Successfully cleared the notifications",
	}, nil
}

// utils
func (s *Server) Getpost(postId int) (*pb.Post, error) {
	var postdetails *pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", postId).Scan(&postdetails).Error; err != nil {
		return nil, errors.New("could not get post from DB")
	}
	return postdetails, nil
}
func (s *Server) GetComments(postId int) ([]*pb.Comment, error) {
	var postComments []*pb.Comment
	if err := s.H.DB.Raw("SELECT * FROM comments where post_id=?", postId).Scan(&postComments).Error; err != nil {
		return nil, errors.New("could not get comments from DB")
	}
	return postComments, nil
}
func (s *Server) Notify(userId int, fromId int, postId int, notificationType string) error {
	text := ""
	if notificationType == "like" {
		text = fmt.Sprintf("You have a new like from %d for post %d", userId, postId)
	} else if notificationType == "comment" {
		text = fmt.Sprintf("You have a new comment from %d for post %d", userId, postId)

	} else if notificationType == "donation" {
		text = fmt.Sprintf("You got a new donation from %d for post %d", userId, postId)
	}
	query := `
    INSERT INTO notifications (user_id,from_id, post_id,time, text, type)
    VALUES (?, ?, ?, ?, ?,?)
	`
	s.H.DB.Raw(query, fromId, userId, postId, time.Now(), text, notificationType)
	return nil
}

// func (s *Server) VerifyPayment(paymentID string, razorID string, orderID string) error {

// 	err := p.paymentRepository.UpdatePaymentDetails(orderID, paymentID, razorID)
// 	if err != nil {
// 		return err
// 	}

// 	//clearcart

// 	orderIDint, err := strconv.Atoi(orderID)
// 	//fmt.Println("====orderID", orderID)
// 	if err != nil {
// 		return err
// 	}
// 	userID, err := p.userRepository.FindUserIDByOrderID(orderIDint)
// 	if err != nil {
// 		return err
// 	}
// 	cartID, err := p.userRepository.GetCartID(userID)
// 	//fmt.Println("CartID=======", cartID)

// 	if err != nil {
// 		return err
// 	}
// 	p.userRepository.ClearCart(cartID)
// 	if err != nil {
// 		return err
// 	}

// 	return nil

// }
