package main

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"database/sql"
	"github.com/brainboxweb/go-posts-utils/bitly"
	_ "github.com/mattn/go-sqlite3"

	"errors"
	"net/url"
	"strings"
)

var database = "../go-posts-admin/db/dtp.db"
var postsFile = "/Users/garystraughan/Sites/dtp/www/data/posts.json"

func main() {

	app := cli.NewApp()
	app.Name = "post utilties"
	app.Usage = "not sure yet!"
	app.Action = func(c *cli.Context) error {

		if c.NArg() > 0 {
			//if c.Args().Get(0) == "backup" {
			//	backup()
			//}

			if c.Args().Get(0) == "generate" {
				generateJSON()
			}

			if c.Args().Get(0) == "refresh-tweets" {
				refreshTweets()
			}
		}
		return nil
	}
	app.Run(os.Args)
}

//
//func generateYML() {
//
//	//readdatabase
//
//	db, err := sql.Open("sqlite3", database)
//	if err != nil {
//		panic(err)
//	}
//	defer db.Close()
//
//
//	//`id` INTEGER PRIMARY KEY AUTOINCREMENT,
//	//`slug` VARCHAR(255) NULL,
//	//`title` VARCHAR(255) NULL,
//	//`description` VARCHAR(400) NULL,
//	//`published` DATETIME NULL,
//	//`body` TEXT,
//	//`transcript` TEXT NULL,
//	//`topresult` TEXT NULL,
//	//`click_to_tweet` VARCHAR(20)
//
//	rows, err := db.Query("SELECT id, slug, title, description, published, body, transcript, topresult, click_to_tweet FROM posts")
//	if err != nil {
//		panic(err)
//	}
//
//	var id int
//	var slug string
//
//	posts := make(map[string]Post)
//	for rows.Next() {
//		post := new(Post)
//
//		err = rows.Scan(&id, &slug, &post.Title, &post.Description, &post.Date, &post.Body, &post.Transcript, &post.TopResult,  &post.ClickToTweet )
//		if err != nil {
//			panic(err)
//		}
//
//		slug = fmt.Sprintf("%d-%s", id, slug)
//		posts[slug] = *post
//	}
//
//	fmt.Println(posts)
//
//
//
//}

//This is the json that "drives" the website
func generateJSON() {

	db, err := sql.Open("sqlite3", database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	posts := make(map[int]Post)

	rows, err := db.Query("SELECT id, slug, title, description, published, body, transcript, coalesce(topresult, '') AS topresult FROM posts")
	if err != nil {
		panic(err)
	}

	for rows.Next() {

		p := new(Post)

		err = rows.Scan(&p.Id, &p.Slug, &p.Title, &p.Description, &p.Date, &p.Body, &p.Transcript, &p.TopResult)
		if err != nil {
			panic(err)
		}

		//Tags/Keywords
		//keywords := new(Keywords)

		rows2, err := db.Query("SELECT keyword_id FROM posts_keywords_xref WHERE post_id = ?", p.Id)
		if err != nil {
			panic(err)
		}

		for rows2.Next() {

			keyword := ""

			err = rows2.Scan(&keyword)
			if err != nil {
				panic(err)
			}

			p.Keywords = append(p.Keywords, keyword)
		}

		//Youtube
		yt := new(YouTubeData)

		rows3, err := db.Query("SELECT id, body FROM youtube WHERE post_id = ?", p.Id)
		if err != nil {
			panic(err)
		}

		for rows3.Next() {
			err = rows3.Scan(&yt.Id, &yt.Body)
			if err != nil {
				panic(err)
			}

			rows4, err := db.Query("SELECT music_id  FROM youtube_music_xref WHERE youtube_id = ?", yt.Id)
			if err != nil {
				panic(err)
			}

			for rows4.Next() {

				var music string
				err = rows4.Scan(&music)
				if err != nil {
					panic(err)
				}

				yt.Music = append(yt.Music, music)
			}

			//Assign to the post
			p.YouTubeData = *yt
		}

		posts[p.Id] = *p

	}

	toJson(posts)
}

func toJson(posts map[int]Post) {

	bytes, err := json.Marshal(posts)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	ioutil.WriteFile(postsFile, bytes, 0644)

}

type Tweet struct {
	PostID string
	Link   string
}

func refreshTweets() {

	fmt.Println("Starting refreshTweets")

	newTweets := []Tweet{}

	db, err := sql.Open("sqlite3", database)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	//posts := make(map[int]Post)

	rows, err := db.Query("SELECT p.id, p.title, p.click_to_tweet, yt.id as yt_id FROM posts p LEFT JOIN youtube yt ON p.id = yt.post_id")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {

		id := ""
		title := ""
		clickToTweet := ""
		ytID := ""

		err = rows.Scan(&id, &title, &clickToTweet, &ytID)
		if err != nil {
			panic(err)
		}

		//Hastags
		rows2, err := db.Query("SELECT hashtag_id FROM posts_hashtags_xref WHERE post_id = ?", id)
		if err != nil {
			panic(err)
		}
		defer rows2.Close()

		hashtags := []string{}
		for rows2.Next() {

			hashtag := ""

			err = rows2.Scan(&hashtag)
			if err != nil {
				panic(err)
			}

			hashtags = append(hashtags, hashtag)
		}
		//Create the tweet String

		target := "https://youtu.be/"+ytID
		handle := "@DevThatPays"
		newClickToTweet, err := buildTweet(title, target, handle, hashtags)
		if err != nil {
			panic("Build tweet failed")
		}


		if newClickToTweet != clickToTweet {
			newTweets = append(newTweets, Tweet{id, newClickToTweet})
		}

	}


	//fmt.Println(newTweets)


	//Hastags
	stmnt, err := db.Prepare("UPDATE posts SET click_to_tweet = ?, click_to_tweet_encoded = ? WHERE id = ?")
	if err != nil {
		panic(err)
	}
	defer stmnt.Close()

	for _, newTweet := range newTweets {

		fmt.Println("Updating Click to Tweet for Post %s", newTweet.PostID)

		tweet := url.QueryEscape(newTweet.Link)

		link := fmt.Sprintf("https://twitter.com/intent/tweet?text=%s", tweet)

		clickToTweetEncoded := bitly.GetShortedLink(link)


		_, err = stmnt.Exec(newTweet.Link, clickToTweetEncoded, newTweet.PostID)
		if err != nil {
			log.Fatal(err)
		}

	}

}

//might be handy
func fetchRemotePostsData() {

	client := &http.Client{}

	resp, err := client.Get("http://www.developmentthatpays.com/" + postsFile)

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	err = ioutil.WriteFile(postsFile, data, 0644)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

//
//
//func getPosts(postsFle string) map[string]Post {
//
//	data := readYAMLFile(postsFle)
//	posts := convertYAML(data)
//
//	return posts
//}
//
//func readYAMLFile(filename string) []byte {
//
//	data, err := ioutil.ReadFile(filename)
//
//	if err != nil {
//		log.Fatalf("Failed to read YML file : %v", err.Error())
//	}
//
//	return data
//}

type YouTubeData struct {
	Id    string   `json:"id"`
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Music []string `json:"music"`
}

type Post struct {
	Id          int         `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Date        string      `json:"date"`
	TopResult   string      `json:"topResult"`
	Keywords    []string    `json:"keywords"`
	YouTubeData YouTubeData `json:"youtubedata"`
	Body        string      `json:"body"`
	Transcript  string      `json:"transcript"`
}

//
//
//func convertYAML(input []byte) map[string]Post {
//	posts := make(map[string]Post)
//
//	err := yaml.Unmarshal(input, &posts)
//	if err != nil {
//		log.Fatalf("error: %v", err)
//	}
//	return posts
//}

//
//func backup() {
//
//	vids := getYouTubeData()
//
//	d, err := yaml.Marshal(&vids)
//	if err != nil {
//		log.Fatalf("error: %v", err)
//	}
//
//	d1 := []byte(d)
//
//	t := time.Now()
//	filename := fmt.Sprintf("backup/youtube-%d-%d-%d.yml", t.Year(), t.Month(), t.Day())
//
//	err = ioutil.WriteFile(filename, d1, 0644)
//	if err != nil {
//		log.Fatalf("error: %v", err)
//	}
//}

//[ARTICLE TITLE]: http://url.com #hashtag by @TwitterHandle

func buildTweet(title, target, twitterHandle string, hashtags []string) (string, error) {

	titleLength := len(title) + 2          //colon + space
	handleLength := len(twitterHandle) + 4 //space+by+space
	linkLength := 23                       //Not sure why

	subTotal := titleLength + handleLength + linkLength
	if subTotal > 140 {
		return "", errors.New("Title too long")
	}

	hashtags = normaliseHashtags(hashtags)

	approvedHastags := []string{}
	for _, hashtag := range hashtags {
		subTotal += len(hashtag) + 1
		if subTotal > 140 {
			break
		}
		approvedHastags = append(approvedHastags, hashtag)
	}

	hashString := strings.Join(approvedHastags, " ")

	tweet := fmt.Sprintf("%s: %s %s by %s", title, target, hashString, twitterHandle)
	tweet = strings.Replace(tweet, "  ", " ", -1) // remove double spaces

	return tweet, nil
}

func normaliseHashtags(hashtags []string) []string{

	for i:=0; i < len(hashtags); i++{
		hashtags[i] = "#"+ strings.Trim(hashtags[i],"# ")
	}

	return hashtags

}