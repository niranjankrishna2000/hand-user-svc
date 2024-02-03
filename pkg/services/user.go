package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
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

func (s *Server) GetCreatePost(ctx context.Context, req *pb.GetCreatePostRequest) (*pb.GetCreatePostResponse, error) {

	log.Println("Category Request List started: ", req)
	var categoryList []*pb.Category
	sqlQuery := "SELECT * FROM categories"
	if err := s.H.DB.Raw(sqlQuery).Scan(&categoryList).Error; err != nil {
		return &pb.GetCreatePostResponse{
			Status:     http.StatusBadGateway,
			Response:   "error from db: " + err.Error(),
			Categories: []*pb.Category{},
		}, errors.New("couldnt fetch categories")
	}
	log.Println("Data Recieved: ", req)
	return &pb.GetCreatePostResponse{
		Status:     http.StatusOK,
		Response:   "Successfully Fetched the data",
		Categories: categoryList,
	}, nil

} //testedok

func (s *Server) CreatePost(ctx context.Context, req *pb.CreatePostRequest) (*pb.CreatePostResponse, error) {

	log.Println("Post creation started: ", req)
	var PostID int32
	layout := "2006-01-02 15:04:05"
	timestamp, err := time.Parse(layout, req.Date)
	if err != nil {
		fmt.Println("Error parsing string:", err)
		return &pb.CreatePostResponse{
			Status:   http.StatusBadRequest,
			Response: "Error Parsing time string",
			Post:     &pb.Post{},
		}, errors.New("invalid date format")
	}
	query := `
    INSERT INTO posts (text, place,image, date, amount,user_id,account_no,address,cat_id,tax_benefit)
    VALUES (?, ?, ?, ?, ?,?,?,?,?,?) RETURNING id
	`
	s.H.DB.Raw(query, req.Text, req.Place, req.Image, timestamp, req.Amount, req.Userid, req.Accno, req.Address, req.Categoryid, req.Taxbenefit).Scan(&PostID)
	var postdetails *pb.Post

	if err := s.H.DB.Raw("SELECT * FROM posts where id=?", PostID).Scan(&postdetails).Error; err != nil {
		return &pb.CreatePostResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB" + err.Error(),
			Post:     &pb.Post{},
		}, errors.New("could not fetch post")
	}
	return &pb.CreatePostResponse{
		Status:   http.StatusCreated,
		Response: "Successfully created post",
		Post:     postdetails,
	}, nil
}

//test image nil
//test category

func (s *Server) UserFeeds(ctx context.Context, req *pb.UserFeedsRequest) (*pb.UserFeedsResponse, error) {

	log.Println("Feeds collection started: ", req)
	// note: check for autopay, try implementing goroutine if exists return link in response.
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
	//select category
	//select type |trending(sort by views and likes)|expired|taxbenefit|
	sqlQuery := "SELECT * FROM posts where id > 0"
	if req.Searchkey != "" {
		sqlQuery += " AND (text ILIKE '%" + req.Searchkey + "%' OR place ILIKE '%" + req.Searchkey + "%' OR title ILIKE '%" + req.Searchkey + "%')"
	}
	if req.Category != 0 {
		sqlQuery += " AND category_id = " + strconv.Itoa(int(req.Category))
	}
	if req.Type == 2 {
		sqlQuery += " AND status = 'expired'"
	} else if req.Type == 3 {
		sqlQuery += " AND tax_benefit = true"
	} else {
		sqlQuery += " AND status = 'approved' AND date >= CURRENT_DATE - INTERVAL '30 days'"
	}
	sqlQuery += " ORDER BY likes DESC, views DESC,date DESC, amount DESC, cat_id DESC LIMIT ? OFFSET ?"
	if err := s.H.DB.Raw(sqlQuery, limit, offset).Scan(&postdetails).Error; err != nil {
		return &pb.UserFeedsResponse{
			Status:         http.StatusBadRequest,
			Response:       "couldn't get posts from DB.",
			Posts:          []*pb.Post{},
			Successstories: []*pb.SuccesStory{},
			Categories:     []*pb.Category{},
		}, errors.New("could not fetch posts")
	}
	var categoryList []*pb.Category
	sqlQuery = "SELECT * FROM categories"
	if err := s.H.DB.Raw(sqlQuery).Scan(&categoryList).Error; err != nil {
		return &pb.UserFeedsResponse{
			Status:         http.StatusBadGateway,
			Response:       "error from db: " + err.Error(),
			Posts:          postdetails,
			Successstories: []*pb.SuccesStory{},
			Categories:     []*pb.Category{},
		}, errors.New("couldnt fetch categories")
	}
	var storyList []*pb.SuccesStory
	sqlQuery = "SELECT * FROM stories ORDER BY date DESC LIMIT ? OFFSET ?"
	if err := s.H.DB.Raw(sqlQuery, limit/2, offset).Scan(&storyList).Error; err != nil {
		return &pb.UserFeedsResponse{
			Status:         http.StatusBadGateway,
			Response:       "error from db: " + err.Error(),
			Posts:          postdetails,
			Successstories: []*pb.SuccesStory{},
			Categories:     categoryList,
		}, errors.New("couldnt fetch stories")
	}
	log.Println("feeds:", postdetails)
	link := s.CheckAutoPay(req.Userid)
	return &pb.UserFeedsResponse{
		Status:         http.StatusOK,
		Response:       "got all records" + link,
		Posts:          postdetails,
		Categories:     categoryList,
		Successstories: storyList,
	}, nil

} //note: add collection of success stories, categories

func (s *Server) UserPostDetails(ctx context.Context, req *pb.UserPostDetailsRequest) (*pb.UserPostDetailsResponse, error) {

	log.Println("Post detailes started",req)

	var post pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts WHERE id=? AND (status = 'approved' OR status ='expired')", req.PostID).Scan(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Println(err.Error())
			return &pb.UserPostDetailsResponse{
				Status:   http.StatusNotFound,
				Response: "Post Not Found",
				Post:     &pb.PostDetails{},
			}, errors.New("post not found")
		}
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
	//note add get updates
	//note add get donations
	return &pb.UserPostDetailsResponse{
		Status:   http.StatusOK,
		Response: "Successfully got the post",
		Post:     postdetails,
	}, nil

}

func (s *Server) Donate(ctx context.Context, req *pb.DonateRequest) (*pb.DonateResponse, error) {

	log.Println("Donation started")
	var postdetails *pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts where id=? and status='approved'", req.Postid).Scan(&postdetails).Error; err != nil {
		return &pb.DonateResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
			Post:     &pb.Post{},
			Link:     "",
		}, err
	}
	date, err := time.Parse(time.RFC3339, postdetails.Date)
	if err != nil {
		return &pb.DonateResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't parse date",
			Post:     &pb.Post{},
			Link:     "",
		}, errors.New("couldn't parse date")
	}
	//test
	if postdetails.Amount <= postdetails.Collected || date.Before(time.Now()) {
		err := s.H.DB.Exec("UPDATE posts SET status = 'expired' WHERE id = ?", req.Postid).Error
		if err != nil {
			return &pb.DonateResponse{
				Status:   http.StatusBadRequest,
				Response: "couldn't update post in DB: " + err.Error(),
				Post:     &pb.Post{},
				Link:     "",
			}, errors.New("couldn't update post in DB")
		}

		return &pb.DonateResponse{
			Status:   http.StatusBadRequest,
			Response: "campaign expired",
			Post:     postdetails,
			Link:     "",
		}, errors.New("campaign expired")
	}
	//create new data and return id
	var payID int32
	query := `
    INSERT INTO payments (user_id, post_id,amount, date,status)
    VALUES (?, ?, ?, ?,?) RETURNING id
	`
	s.H.DB.Raw(query, req.Userid, req.Postid, req.Amount, time.Now(), "pending").Scan(&payID)

	link := fmt.Sprintf("https://handcrowdfunding.online/user/post/donate/razorpay?payid=%d", payID)

	return &pb.DonateResponse{
		Status:   http.StatusOK,
		Response: "Click on the link to donate",
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
			Response: "Payment Not Available:" + err.Error(),
		}, errors.New("payment unavaliable")
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
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: "could not update payment details" + err.Error()}, errors.New("payment comeplete. error when updating payments")
	}
	post, err := s.CheckPostId(int32(paymentdetail.PostID))
	if err != nil {
		fmt.Println(err)
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: "notification error: " + err.Error()}, errors.New("could not send notification")

	}
	err = s.Notify(int(post.UserId), paymentdetail.UserID, paymentdetail.PostID, "donation")
	if err != nil {
		fmt.Println(err)
		return &pb.MakePaymentRazorPayResponse{Status: http.StatusBadGateway, Response: "notification error: " + err.Error()}, errors.New("could not send notification")

	}
	return &postDetails, nil
}

func (s *Server) GenerateInvoice(ctx context.Context, req *pb.GenerateInvoiceRequest) (*pb.GenerateInvoiceResponse, error) {

	var paymentdetail models.Payment
	var address string
	if err := s.H.DB.Raw("SELECT * FROM payments WHERE payment_id=? AND status = 'completed'", req.InvoiceId).Scan(&paymentdetail).Error; err != nil {
		return &pb.GenerateInvoiceResponse{
			Status:   http.StatusBadRequest,
			Response: "Payment Data Not Available:" + err.Error(),
		}, errors.New("payment data not found")
	}
	if err := s.H.DB.Raw("SELECT CONCAT(account_no, ' ', address) FROM posts WHERE id=?", paymentdetail.PostID).Scan(&address).Error; err != nil {
		return &pb.GenerateInvoiceResponse{
			Status:   http.StatusBadRequest,
			Response: "Payment address Data Not Available:" + err.Error(),
		}, errors.New("payment address Data Not Available")
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
	_, err := s.CheckPostId(req.Postid)
	if err != nil {
		return &pb.ReportPostResponse{Status: http.StatusBadGateway, Response: "Invalid post id"}, errors.New("invalid post id")
	}

	// }
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
			Response: "couldn't get posts from DB: " + err.Error(),
			Post:     &pb.Post{},
		}, errors.New("couldn't get post")
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

	postId := 0
	query := `
    INSERT INTO reporteds(reason,user_id,comment_id,category)
    VALUES(?, ?, ?, 'comment')
	`
	log.Println("inserting into reportlist")
	if err := s.H.DB.Exec(query, req.Text, req.Userid, req.Commentid).Error; err != nil {
		log.Printf("Failed to insert report: %v", err)
		return &pb.ReportCommentResponse{
			Status:   http.StatusInternalServerError,
			Response: "Failed to insert report:" + err.Error(),
			Post:     &pb.PostDetails{},
		}, errors.New("failed to insert report")
	}
	log.Println("Fetching post id")
	if err := s.H.DB.Raw("SELECT post_id FROM comments where id=?", req.Commentid).Scan(&postId).Error; err != nil || postId == 0 {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get postid from DB",
			Post:     &pb.PostDetails{},
		}, errors.New("could not get postid from DB")
	}
	var postdetails *pb.PostDetails

	if err := s.H.DB.Exec("SELECT * FROM posts where id=?", postId).Scan(&postdetails.Post).Error; err != nil {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get post from DB",
		}, errors.New("could not get post from DB")
	}

	if err := s.H.DB.Exec("SELECT * FROM comments where post_id=?", postId).Scan(&postdetails.Comments).Error; err != nil {
		return &pb.ReportCommentResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get comment from DB",
			Post:     postdetails,
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

	var postdetails *pb.Post
	postdetails, err := s.CheckPostId(req.Postid)
	if err != nil {
		return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Invalid post id"}, errors.New("invalid post id")
	}

	if !s.CheckIfOwner(req.Userid, req.Postid) {
		return &pb.EditPostResponse{Status: http.StatusUnauthorized, Response: "Unauthorized"}, errors.New("Unauthorized")
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
				Post:     &pb.Post{},
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
	if req.Title != "" {
		err := s.H.DB.Exec("UPDATE posts set title = ? where id = ?", req.Title, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update title"}, err
		}
	}
	if req.Amount >= postdetails.Collected {
		err := s.H.DB.Exec("UPDATE posts set amount = ? where id = ?", req.Amount, req.Postid).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditPostResponse{Status: http.StatusBadGateway, Response: "Could not Update Amount"}, err
		}
	} else {
		return &pb.EditPostResponse{Status: http.StatusBadRequest, Response: "Amount should be more than collected"}, err
	}

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
	err = s.Notify(int(Post.UserId), int(req.Userid), int(Post.Id), "like")
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
		return &pb.DeleteCommentResponse{Status: http.StatusBadGateway, Response: "Could not get post from db"}, err
	}
	Comments, err := s.GetComments(int(postId))
	if err != nil {
		return &pb.DeleteCommentResponse{Status: http.StatusBadGateway, Response: "Could not get comments from db", Post: &pb.PostDetails{Post: Post}}, err
	}
	return &pb.DeleteCommentResponse{
		Status:   http.StatusOK,
		Response: "Successfully removed the comment",
		Post:     &pb.PostDetails{Post: Post, Comments: Comments},
	}, nil
}

////////////////////////////////Test till here////////////////////////////////////////////////////////////////////////

// donation
func (s *Server) DonationHistory(ctx context.Context, req *pb.DonationHistoryRequest) (*pb.DonationHistoryResponse, error) {
	log.Println("Donation History started",req)
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

	var donations []*pb.Donation
	var donationlist []*models.Payment
	
	sqlQuery := "SELECT * FROM payments WHERE status = 'completed' and user_id=?"
	sqlQuery += " ORDER BY date DESC, amount DESC LIMIT ? OFFSET ?"

	if err := s.H.DB.Raw(sqlQuery, req.Userid, limit, offset).Scan(&donationlist).Error; err != nil {
		return &pb.DonationHistoryResponse{
			Status:    http.StatusBadRequest,
			Response:  "couldn't get posts from DB",
			Donations: []*pb.Donation{},
		}, err
	}
	for _,value:=range donationlist{
		donations=append(donations, &pb.Donation{Id: int32(value.Id),Date: value.Date.String(),Amount: int64(value.Amount),Paymentid: value.PaymentID})
	}
	fmt.Println(donationlist,donations)
	//iterate and copy
	return &pb.DonationHistoryResponse{
		Status:    http.StatusOK,
		Response:  "successfully retrieved Donation history",
		Donations: donations,
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

func (s *Server) ProfileDetails(ctx context.Context, req *pb.ProfileDetailsRequest) (*pb.ProfileDetailsResponse, error) {
	log.Println("ProfileDetails Service Starting...", req)

	var user pb.UserProfile
	if err := s.H.DB.Raw("SELECT * FROM users WHERE id=? AND (status = 'approved' OR status ='expired')", req.Userid).Scan(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Println(err.Error())
			return &pb.ProfileDetailsResponse{
				Status:   http.StatusNotFound,
				Response: "user Not Found",
				User:     &pb.UserProfile{Name: "", Email: "", Phone: "", Id: user.Id, Gender: "", DoB: "", Address: "", PAN: "", ProfilePicture: ""},
				}, errors.New("user not found")
		}
	}
	log.Println("user profile: ", &user)
	return &pb.ProfileDetailsResponse{
		Status:   http.StatusOK,
		Response: "Successfully got the  records",
		User:     &pb.UserProfile{Name: user.Name, Email: user.Email, Phone: user.Phone, Id: user.Id, Gender: user.Gender, DoB: user.DoB, Address: user.Address, PAN: user.PAN, ProfilePicture: user.ProfilePicture},
	}, nil
}

func (s *Server) EditProfile(ctx context.Context, req *pb.UserProfile) (*pb.EditProfileResponse, error) {
	log.Println("EditProfile Service Starting...", req)
	if req.Name != "" {
		err := s.H.DB.Exec("UPDATE users set name = ? where id = ?", req.Name, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update name", User: &pb.UserProfile{}}, err
		}
	}
	if req.Email != "" {
		err := s.H.DB.Exec("UPDATE users set email = ? where id = ?", req.Email, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update email", User: &pb.UserProfile{}}, err
		}
	}
	if req.Gender != "" {
		err := s.H.DB.Exec("UPDATE users set gender = ? where id = ?", req.Gender, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update gender", User: &pb.UserProfile{}}, err
		}
	}
	if req.Address != "" {
		err := s.H.DB.Exec("UPDATE users set address = ? where id = ?", req.Address, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update Address", User: &pb.UserProfile{}}, err
		}
	}
	if req.PAN != "" {
		err := s.H.DB.Exec("UPDATE users set pan = ? where id = ?", req.PAN, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update Pan No", User: &pb.UserProfile{}}, err
		}
	}
	if req.Phone != "" {
		err := s.H.DB.Exec("UPDATE users set phone = ? where id = ?", req.Phone, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update phone No", User: &pb.UserProfile{}}, err
		}
	}
	if req.DoB != "" {
		layout := "2006-01-02"
		timestamp, err := time.Parse(layout, req.DoB)
		if err != nil {
			fmt.Println("Error parsing string:", err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update DoB No", User: &pb.UserProfile{}}, err
		}
		err = s.H.DB.Exec("UPDATE users set dob = ? where id = ?", timestamp, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update phone No", User: &pb.UserProfile{}}, err
		}
	}
	if req.ProfilePicture != "" {
		err := s.H.DB.Exec("UPDATE users set profile_picture = ? where id = ?", req.ProfilePicture, req.Id).Error
		if err != nil {
			fmt.Println(err)
			return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not Update Profile pic", User: &pb.UserProfile{}}, err
		}
	}
	userProfile := pb.UserProfile{}
	if err := s.H.DB.Raw("SELECT * FROM users where id=?", req.Id).Scan(&userProfile).Error; err != nil {
		return &pb.EditProfileResponse{Status: http.StatusBadGateway, Response: "Could not get user  details", User: &pb.UserProfile{}}, err
	}
	return &pb.EditProfileResponse{
		Status:   http.StatusOK,
		Response: "Successfully got the details",
		User:     &userProfile,
	}, nil
}

// updates
func (s *Server) GetUpdates(ctx context.Context, req *pb.GetUpdatesRequest) (*pb.GetUpdatesResponse, error) {
	_, err := s.CheckPostId(req.Postid)
	if err != nil {
		return &pb.GetUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "Invalid post Id",
		}, errors.New("invalid Post Id")
	}
	updates := []*pb.Update{}
	if err := s.H.DB.Raw("SELECT * FROM updates where id=?", req.Postid).Scan(&updates).Error; err != nil {
		return &pb.GetUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get updates from DB",
		}, errors.New("could not get updates from DB")
	}
	log.Println("GetUpdates Service Starting...", req)
	return &pb.GetUpdatesResponse{
		Status:   http.StatusOK,
		Response: "Successfully collected the updates",
		Updates:  updates,
	}, nil
}
func (s *Server) AddUpdate(ctx context.Context, req *pb.AddUpdatesRequest) (*pb.AddUpdatesResponse, error) {
	log.Println("AddUpdate Service Starting...", req)

	if !s.CheckIfOwner(req.Userid, req.Postid) {
		return &pb.AddUpdatesResponse{
			Status:   http.StatusUnauthorized,
			Response: "You cant add updates to others posts",
		}, errors.New("unauthorized")
	}

	PostID := 0
	query := `
    INSERT INTO updates (text, title,date, postid)
    VALUES (?, ?, ?, ?) RETURNING id
	`
	s.H.DB.Raw(query, req.Text, req.Title, time.Now(), req.Postid).Scan(&PostID)
	var updates []*pb.Update

	if err := s.H.DB.Raw("SELECT * FROM updates where postid=?", req.Postid).Scan(&updates).Error; err != nil {
		return &pb.AddUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB" + err.Error(),
			Updates:  []*pb.Update{},
		}, errors.New("could not fetch post")
	}
	return &pb.AddUpdatesResponse{
		Status:   http.StatusOK,
		Response: "Successfully got Updates",
		Updates:  updates,
	}, nil
}
func (s *Server) EditUpdate(ctx context.Context, req *pb.EditUpdatesRequest) (*pb.EditUpdatesResponse, error) {
	log.Println("EditUpdate Service Starting...", req)
	update := pb.Update{}
	if err := s.H.DB.Raw("SELECT * FROM updates where id =? and user_id=?", req.Updateid, req.Userid).Scan(&update).Error; err != nil {
		return &pb.EditUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "You cant edit the update",
		}, errors.New("could not edit update from DB . post doesnt exist or unauthorized")
	}
	err := s.H.DB.Exec("UPDATE updates set text = ? and title= ? and date =? where id = ?", req.Text, req.Title, time.Now(), req.Updateid).Error
	if err != nil {
		fmt.Println(err)
		return &pb.EditUpdatesResponse{Status: http.StatusBadGateway, Response: "Could not edit the update details"}, err
	}

	updates := pb.Update{}
	if err := s.H.DB.Raw("SELECT * FROM updates where id=?", req.Updateid).Scan(&updates).Error; err != nil {
		return &pb.EditUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get updates from DB",
		}, errors.New("could not get updates from DB")
	}
	return &pb.EditUpdatesResponse{
		Status:   http.StatusOK,
		Response: "Successfully edited the update",
		Updates:  []*pb.Update{&updates},
	}, nil
}
func (s *Server) DeleteUpdate(ctx context.Context, req *pb.DeleteUpdatesRequest) (*pb.DeleteUpdatesResponse, error) {
	log.Println("DeleteUpdate Service Starting...", req)

	update := pb.Update{}
	if err := s.H.DB.Raw("SELECT * FROM updates where id =? and user_id=?", req.Updateid, req.Userid).Scan(&update).Error; err != nil {
		return &pb.DeleteUpdatesResponse{
			Status:   http.StatusBadRequest,
			Response: "You cant delete the update",
		}, errors.New("could not delete update from DB . post doesnt exist or unauthorized")
	}
	err := s.H.DB.Exec("delete from updates where id=?", req.Updateid).Error
	if err != nil {
		return &pb.DeleteUpdatesResponse{Status: http.StatusBadGateway, Response: "Could not remove the update"}, err
	}
	return &pb.DeleteUpdatesResponse{
		Status:   http.StatusOK,
		Response: "Successfully deleted the update",
	}, nil
}

// monthly goals
func (s *Server) GetMonthlyGoal(ctx context.Context, req *pb.GetMonthlyGoalRequest) (*pb.GetMonthlyGoalResponse, error) {
	log.Println("GetMonthlyGoal Service Starting...", req)
	monthly_goals := models.MonthlyGoal{}
	if err := s.H.DB.Raw("SELECT * FROM monthly_goals where user_id=?", req.Userid).Scan(&monthly_goals).Error; err != nil {
		return &pb.GetMonthlyGoalResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get updates from DB",
			Day:      0,
			Amount:   0,
		}, errors.New("could not get updates from DB")
	}
	if monthly_goals.Amount == 0 {
		return &pb.GetMonthlyGoalResponse{
			Status:   http.StatusOK,
			Response: "Set up goals first",
		}, nil
	}
	return &pb.GetMonthlyGoalResponse{
		Status:   http.StatusOK,
		Response: "Successfully collected the Monthly goal",
		Day:      int32(monthly_goals.Day),
		Amount:   int64(monthly_goals.Amount),
	}, nil
}
func (s *Server) AddMonthlyGoal(ctx context.Context, req *pb.AddMonthlyGoalRequest) (*pb.AddMonthlyGoalResponse, error) {
	log.Println("AddMonthlyGoal Service Starting...", req)
	query := `
    INSERT INTO monthly_goals (user_id, amount,day, category)
    VALUES (?, ?, ?, ?)
	`

	s.H.DB.Exec(query, req.Userid, req.Amount, req.Day, req.Category)
	var monthlyGoal models.MonthlyGoal

	if err := s.H.DB.Raw("SELECT * FROM monthly_goals where user_id=?", req.Userid).Scan(&monthlyGoal).Error; err != nil {
		return &pb.AddMonthlyGoalResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB" + err.Error(),
			Category: 0,
			Amount:   0,
			Day:      0,
		}, errors.New("could not fetch post")
	}
	return &pb.AddMonthlyGoalResponse{
		Status:   http.StatusOK,
		Response: "Successfully set the Monthly Goal",
		Amount:   int64(monthlyGoal.Amount),
		Day:      int32(monthlyGoal.Day),
		Category: int32(monthlyGoal.Category),
	}, nil
}
func (s *Server) EditMonthlyGoal(ctx context.Context, req *pb.EditMonthlyGoalRequest) (*pb.EditMonthlyGoalResponse, error) {
	log.Println("EditMonthlyGoal Service Starting...", req)
	var monthlyGoal models.MonthlyGoal

	if err := s.H.DB.Raw("SELECT * FROM monthly_goals where user_id=?", req.Userid).Scan(&monthlyGoal).Error; err != nil || monthlyGoal.Amount == 0 {
		query := `
    	INSERT INTO monthly_goals (user_id, amount,day, category)
    	VALUES (?, ?, ?, ?)
		`
		s.H.DB.Raw(query, req.Userid, req.Amount, req.Day, req.Category)
		return &pb.EditMonthlyGoalResponse{
			Status:   http.StatusBadRequest,
			Response: "added the goal",
			Category: req.Category,
			Amount:   req.Amount,
			Day:      req.Day,
		}, errors.New("added the goal")
	}
	err := s.H.DB.Exec("UPDATE monthly_goals set amount = ? and day= ? and category =? where user_id = ?", req.Amount, req.Day, req.Category, req.Userid).Error
	if err != nil {
		fmt.Println(err)
		return &pb.EditMonthlyGoalResponse{Status: http.StatusBadGateway, Response: "Could not Update the goal details"}, err
	}
	if err := s.H.DB.Raw("SELECT * FROM monthly_goals where user_id=?", req.Userid).Scan(&monthlyGoal).Error; err != nil {
		return &pb.EditMonthlyGoalResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get updates from DB",
			Day:      0,
			Amount:   0,
		}, errors.New("could not get updates from DB")
	}
	return &pb.EditMonthlyGoalResponse{
		Status:   http.StatusOK,
		Response: "Successfully updated the goal",
		Category: int32(monthlyGoal.Category),
		Day:      int32(monthlyGoal.Day),
		Amount:   int64(monthlyGoal.Amount),
	}, nil
}

// My impact
func (s *Server) GetmyImpact(ctx context.Context, req *pb.GetmyImpactRequest) (*pb.GetmyImpactResponse, error) {
	log.Println("GetmyImpact Service Starting...", req)
	likes, views, collected, donated, lives := 0, 0, 0, 0, 0
	if err := s.H.DB.Raw(`SELECT COUNT(*)
	FROM likes
	WHERE userid = ?`, req.UserId).Scan(&likes).Error; err != nil {
		return &pb.GetmyImpactResponse{
			Status:   http.StatusBadRequest,
			Response: "could not get data",
		}, err
	}
	if err := s.H.DB.Raw(`SELECT SUM(views)
	FROM posts
	WHERE user_id = ?`, req.UserId).Scan(&views).Error; err != nil {
		return &pb.GetmyImpactResponse{
			Status:   http.StatusBadRequest,
			Response: "could not get data",
			Likes:    int32(likes),
		}, err
	}
	if err := s.H.DB.Raw(`SELECT SUM(collected)
	FROM posts
	WHERE userid = ?`, req.UserId).Scan(&collected).Error; err != nil {
		return &pb.GetmyImpactResponse{
			Status:   http.StatusBadRequest,
			Response: "could not get data",
			Likes:    int32(likes),
			Views:    int32(views),
		}, err
	}
	if err := s.H.DB.Raw(`SELECT SUM(amount)
	FROM payments
	WHERE userid = ?`, req.UserId).Scan(&donated).Error; err != nil {
		return &pb.GetmyImpactResponse{
			Status:    http.StatusBadRequest,
			Response:  "could not get data",
			Likes:     int32(likes),
			Views:     int32(views),
			Collected: int64(collected),
		}, err
	}
	if err := s.H.DB.Raw(`SELECT COUNT(*)
	FROM posts
	WHERE user_id = ?`, req.UserId).Scan(&lives).Error; err != nil {
		return &pb.GetmyImpactResponse{
			Status:    http.StatusBadRequest,
			Response:  "could not get data",
			Likes:     int32(likes),
			Views:     int32(views),
			Collected: int64(collected),
			Donated:   int64(donated),
		}, err
	}

	return &pb.GetmyImpactResponse{
		Status:       http.StatusOK,
		Response:     "Successfully got all data",
		Likes:        int32(likes),
		Views:        int32(views),
		Collected:    int64(collected),
		Donated:      int64(donated),
		LifesChanged: int32(lives),
	}, nil
}
func (s *Server) GetMyCampaigns(ctx context.Context, req *pb.GetMyCampaignsRequest) (*pb.GetMyCampaignsResponse, error) {
	log.Println("GetMyCampaigns Service Starting...", req)
	Posts := []*pb.Post{}
	if err := s.H.DB.Raw("SELECT * FROM posts where user_id=? and status != 'rejected'", req.UserId).Scan(&Posts).Error; err != nil {
		return &pb.GetMyCampaignsResponse{
			Status:   http.StatusBadRequest,
			Response: "couldn't get posts from DB",
			Posts:    []*pb.Post{},
		}, errors.New("could not get posts from DB")
	}
	log.Println("campaigns",Posts)
	if len(Posts) == 0 {
		return &pb.GetMyCampaignsResponse{
			Status:   http.StatusBadRequest,
			Response: "Create a post first",
			Posts:    []*pb.Post{},
		}, errors.New("no posts found")
	}
	return &pb.GetMyCampaignsResponse{
		Status:   http.StatusOK,
		Response: "Successfully got the posts",
		Posts:    Posts,
	}, nil
}

// success story
func (s *Server) GetSuccessStory(ctx context.Context, req *pb.GetSuccessStoryRequest) (*pb.GetSuccessStoryResponse, error) {
	log.Println("GetSuccessStory Service Starting...", req)
	stories := []*pb.SuccesStory{}
	if err := s.H.DB.Raw("SELECT * FROM stories").Scan(&stories).Error; err != nil {
		return &pb.GetSuccessStoryResponse{
			Status:         http.StatusBadRequest,
			Response:       "couldn't get stories from DB",
			SuccessStories: []*pb.SuccesStory{},
		}, errors.New("could not get stories from DB")
	}
	log.Println("GetStories Service Starting...", req)
	return &pb.GetSuccessStoryResponse{
		Status:         http.StatusOK,
		Response:       "Successfully collected the stories",
		SuccessStories: stories,
	}, nil
}
func (s *Server) AddSuccessStory(ctx context.Context, req *pb.AddSuccessStoryRequest) (*pb.AddSuccessStoryResponse, error) {
	log.Println("AddSuccessStory Service Starting...", req)
	storyId := 0
	query := `
    INSERT INTO stories (title,text, place,image, date,user_id)
    VALUES (?, ?, ?, ?, ?,?) RETURNING id
	`
	s.H.DB.Raw(query, req.Title, req.Text, req.Place, req.Image, time.Now(), req.UserId).Scan(&storyId)
	var stories *pb.SuccesStory

	if err := s.H.DB.Raw("SELECT * FROM stories where id=?", storyId).Scan(&stories).Error; err != nil {
		return &pb.AddSuccessStoryResponse{
			Status:       http.StatusBadRequest,
			Response:     "couldn't get stories from DB" + err.Error(),
			SuccessStory: &pb.SuccesStory{},
		}, errors.New("could not fetch post")
	}
	return &pb.AddSuccessStoryResponse{
		Status:       http.StatusOK,
		Response:     "Successfully created the success story",
		SuccessStory: stories,
	}, nil
}
func (s *Server) EditSuccessStory(ctx context.Context, req *pb.EditSuccessStoryRequest) (*pb.EditSuccessStoryResponse, error) {
	log.Println("EditSuccessStory Service Starting...", req)
	story := pb.SuccesStory{}
	if err := s.H.DB.Raw("SELECT * FROM stories where id=? and user_id=?", req.Storyid, req.UserId).Scan(&story).Error; err != nil {
		return &pb.EditSuccessStoryResponse{
			Status:       http.StatusBadRequest,
			Response:     "Invalid story. You are unauthorized or wrong id",
			SuccessStory: &pb.SuccesStory{},
		}, errors.New("could not get story from DB")
	}
	err := s.H.DB.Exec("UPDATE stories set title = ? and text= ? and place =? and image=? where id = ?", req.Title, req.Text, req.Place, req.Image, req.Storyid).Error
	if err != nil {
		fmt.Println(err)
		return &pb.EditSuccessStoryResponse{Status: http.StatusBadGateway, Response: "Could not Update the success story details"}, err
	}
	if err := s.H.DB.Raw("SELECT * FROM stories where id=?", req.Storyid).Scan(&story).Error; err != nil {
		return &pb.EditSuccessStoryResponse{
			Status:       http.StatusBadRequest,
			Response:     "could not get updated story from db",
			SuccessStory: &pb.SuccesStory{},
		}, errors.New("could not get story from DB")
	}
	return &pb.EditSuccessStoryResponse{
		Status:       http.StatusOK,
		Response:     "Successfully updated the success story",
		SuccessStory: &story,
	}, nil
}
func (s *Server) DeleteSuccessStory(ctx context.Context, req *pb.DeleteSuccessStoryRequest) (*pb.DeleteSuccessStoryResponse, error) {
	log.Println("DeleteSuccessStory Service Starting...", req)
	story := pb.SuccesStory{}
	if err := s.H.DB.Raw("SELECT * FROM stories where id =? and user_id=?", req.Storyid, req.Userid).Scan(&story).Error; err != nil {
		return &pb.DeleteSuccessStoryResponse{
			Status:         http.StatusBadRequest,
			Response:       "No such story exist",
			SuccessStories: []*pb.SuccesStory{},
		}, errors.New("could not delete stories from DB . post doenst exist")
	}
	err := s.H.DB.Exec("delete from stories where id=?", req.Storyid).Error
	if err != nil {
		return &pb.DeleteSuccessStoryResponse{Status: http.StatusBadGateway, Response: "Could not remove the story", SuccessStories: []*pb.SuccesStory{}}, err
	}
	stories := []*pb.SuccesStory{}
	if err := s.H.DB.Raw("SELECT * FROM stories").Scan(&stories).Error; err != nil {
		return &pb.DeleteSuccessStoryResponse{
			Status:         http.StatusBadRequest,
			Response:       "couldn't get stories from DB",
			SuccessStories: []*pb.SuccesStory{},
		}, errors.New("could not get stories from DB")
	}
	return &pb.DeleteSuccessStoryResponse{
		Status:         http.StatusOK,
		Response:       "Successfully removed the story",
		SuccessStories: stories,
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
	log.Println("==================notification====================")
	text := ""
	text = fmt.Sprintf("You have a new %s from %d for post %d", notificationType, userId, postId)
	query := `
    INSERT INTO notifications(user_id,from_id, post_id,time, text, type)
    VALUES (?, ?, ?, ?, ?,?)
	`
	s.H.DB.Exec(query, fromId, userId, postId, time.Now(), text, notificationType)
	return nil
}
func (s *Server) CheckUserId(userId int32) (*pb.User, error) {
	var user pb.User

	if result := s.H.DB.Where(&pb.User{Id: int32(userId)}).First(&user); result.Error != nil {
		log.Println(result.Error)
		return nil, errors.New("user not found")
	}
	return &user, nil
}
func (s *Server) CheckPostId(postId int32) (*pb.Post, error) {
	var post pb.Post

	if result := s.H.DB.Where(&pb.Post{Id: int32(postId)}).First(&post); result.Error != nil {
		log.Println(result.Error)
		return nil, errors.New("post not found")
	}
	if post.Id == 0 {
		return nil, errors.New("post not found")

	}
	return &post, nil
}
func (s *Server) CheckIfOwner(userId, postId int32) bool {
	var post pb.Post
	if result := s.H.DB.Where("id = ? AND user_id = ?", postId, userId).First(&post); result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false
		}
		log.Println(result.Error)
		return false
	}
	return true
}

func (s *Server) CheckAutoPay(userId int32) string {
	var monthlyGoal models.MonthlyGoal
	if err := s.H.DB.Raw("SELECT * FROM monthly_goals WHERE user_id=?", userId).Scan(&monthlyGoal).Error; err != nil {
		return ""
	}
	if monthlyGoal.Day < time.Now().Day() {
		return ""
	}
	var post pb.Post
	if err := s.H.DB.Raw("SELECT * FROM posts WHERE cat_id=? AND status = 'approved' AND amount - collected < ?)", monthlyGoal.Category, monthlyGoal.Amount).Scan(&post).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Println(err.Error())
			return ""
		}
	}
	var payID int32
	query := `
    INSERT INTO payments(user_id, post_id,amount, date,status)
    VALUES (?, ?, ?, ?,?) RETURNING id
	`
	s.H.DB.Raw(query, userId, post.Id, monthlyGoal.Amount, time.Now(), "pending").Scan(&payID)

	link := fmt.Sprintf("https://handcrowdfunding.online/user/post/donate/razorpay?payid=%d", payID)
	return "Pay This Month Donation Today :" + link
}

// func (s *Server) GetUpdates() {}
// func (s *Server) GetStories() {}
