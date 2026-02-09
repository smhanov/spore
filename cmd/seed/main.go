package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	blog "github.com/smhanov/spore"
	"github.com/yuin/goldmark"
)

var budgiePosts = []struct {
	Title    string
	Markdown string
	Tags     []string
}{
	{
		Title: "The Ultimate Guide to Budgie Diet",
		Tags:  []string{"diet", "health"},
		Markdown: `Feeding your budgie a balanced diet is crucial for their health.

## Seeds vs Pellets
While seeds are a traditional favorite, they are high in fat. Pellets should form the base of the diet.

## Fresh Vegetables
Incorporate leafy greens like spinach and romaine lettuce. Carrots and broccoli are also great.

## What to Avoid
Never feed your budgie avocado, chocolate, or caffeine.`,
	},
	{
		Title: "Top 5 Toys for Your Budgie",
		Tags:  []string{"toys", "enrichment"},
		Markdown: `Budgies are intelligent birds that need stimulation.

1. **Mirrors**: Budgies love interacting with their reflection (but use in moderation).
2. **Bells**: The noise is very satisfying for them.
3. **Swings**: A classic for a reason.
4. **Shredding Toys**: Great for their beak health.
5. **Puzzle Toys**: Hide treats inside to keep them busy.`,
	},
	{
		Title: "How to Teach Your Budgie to Talk",
		Tags:  []string{"training", "behavior"},
		Markdown: `Budgies are among the best talkers in the parrot world.

## Repetition is Key
Choose a simple phrase like "Pretty bird" and repeat it clearly and often.

## Positive Reinforcement
Reward them with millet when they make an attempt.

## Patience
Some budgies pick it up in weeks, others take months. Keep at it!`,
	},
	{
		Title: "Understanding Budgie Colors and Mutations",
		Tags:  []string{"genetics", "colors"},
		Markdown: `Budgies come in a rainbow of colors.

- **Green Series**: The wild type, dominant gene.
- **Blue Series**: A common recessive mutation.
- **Lutino**: Pure yellow with red eyes.
- **Albino**: Pure white with red eyes.

Genetics can be complex but fascinating!`,
	},
	{
		Title: "Choosing the Right Cage Size",
		Tags:  []string{"housing", "care"},
		Markdown: `A bigger cage is always better.

## Minimum Dimensions
For a single budgie, aim for at least 18x18x18 inches.

## Bar Spacing
Ensure bar spacing is no more than 1/2 inch to prevent escape or injury.

## Orientation
Horizontal space is more important than vertical space for flying.`,
	},
	{
		Title: "Signs of a Healthy Budgie",
		Tags:  []string{"health", "care"},
		Markdown: `Knowing what to look for can save your bird's life.

- **Bright Eyes**: Clear and alert.
- **Smooth Feathers**: Clean and well-preened.
- **Active Behavior**: Chirping and playing.
- **Clean Vent**: No staining around the tail.

If you notice puffing up or lethargy, see a vet immediately.`,
	},
	{
		Title: "Bonding with Your New Budgie",
		Tags:  []string{"bonding", "training"},
		Markdown: `Building trust takes time.

1. **Give them space** for the first few days.
2. **Talk gently** to them through the cage.
3. **Offer treats** like millet through the bars.
4. **Hand taming**: Slowly introduce your hand inside the cage.

Consistency builds the strongest bond.`,
	},
	{
		Title: "Deciphering Budgie Sounds",
		Tags:  []string{"behavior", "sounds"},
		Markdown: `What is your bird trying to say?

- **Chirping**: Happy and content.
- **Squawking**: Annoyed or demanding attention.
- **Beak Grinding**: A sign of relaxation, usually before sleep.
- **Hissing**: Threatened or aggressive.`,
	},
	{
		Title: "How Long Do Budgies Live?",
		Tags:  []string{"health", "lifespan"},
		Markdown: `In captivity, budgies typically live between 5 to 10 years, though some reach 15!

## Factors Influencing Longevity
- **Diet**: High quality nutrition extends life.
- **Exercise**: Flight time is essential.
- **Genetics**: Breeding plays a role.
- **Veterinary Care**: Regular checkups help preventing issues.`,
	},
	{
		Title: "Budgie Sleep Requirements",
		Tags:  []string{"health", "care"},
		Markdown: `Budgies need plenty of rest to stay healthy.

## Hours Needed
They require 10-12 hours of uninterrupted sleep each night.

## Cover the Cage
Use a cage cover to create a dark, quiet environment.

## Night Frights
Some budgies are prone to night frights; a small night light can help.`,
	},
}

func main() {
	dbPath := "blog.db"
	fmt.Printf("Opening database at %s...\n", dbPath)

	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	store := blog.NewSQLXStore(db)
	if err := store.Migrate(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	fmt.Println("Building WXR document...")
	wxrPayload, err := buildSeedWXR()
	if err != nil {
		log.Fatalf("Failed to build WXR: %v", err)
	}

	fmt.Println("Importing via WXR...")
	if err := blog.ImportWXRData(context.Background(), store, wxrPayload); err != nil {
		log.Fatalf("WXR import failed: %v", err)
	}

	fmt.Println("Done!")
}

// buildSeedWXR generates a WXR XML document from the built-in budgie posts.
func buildSeedWXR() ([]byte, error) {
	type wxrCategory struct {
		XMLName  xml.Name `xml:"category"`
		Domain   string   `xml:"domain,attr"`
		Nicename string   `xml:"nicename,attr"`
		Name     string   `xml:",cdata"`
	}
	type wxrItem struct {
		Title          string        `xml:"title"`
		ContentEncoded string        `xml:"content:encoded"`
		PostDateGMT    string        `xml:"wp:post_date_gmt"`
		PostName       string        `xml:"wp:post_name"`
		Status         string        `xml:"wp:status"`
		PostType       string        `xml:"wp:post_type"`
		Categories     []wxrCategory `xml:"category,omitempty"`
	}
	type wxrChannel struct {
		Title string    `xml:"title"`
		Items []wxrItem `xml:"item"`
	}
	type wxrRSS struct {
		XMLName   xml.Name   `xml:"rss"`
		Version   string     `xml:"version,attr"`
		ContentNS string     `xml:"xmlns:content,attr"`
		WPNS      string     `xml:"xmlns:wp,attr"`
		Channel   wxrChannel `xml:"channel"`
	}

	items := make([]wxrItem, 0, len(budgiePosts))
	for i, bp := range budgiePosts {
		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(bp.Markdown), &buf); err != nil {
			return nil, fmt.Errorf("convert markdown for %q: %w", bp.Title, err)
		}
		pubDate := time.Now().Add(time.Duration(-i) * 24 * time.Hour).UTC()

		cats := make([]wxrCategory, 0, len(bp.Tags))
		for _, t := range bp.Tags {
			cats = append(cats, wxrCategory{
				Domain:   "post_tag",
				Nicename: t,
				Name:     t,
			})
		}

		items = append(items, wxrItem{
			Title:          bp.Title,
			ContentEncoded: buf.String(),
			PostDateGMT:    pubDate.Format("2006-01-02 15:04:05"),
			PostName:       fmt.Sprintf("budgie-post-%d", i+1),
			Status:         "publish",
			PostType:       "post",
			Categories:     cats,
		})
	}

	rss := wxrRSS{
		Version:   "2.0",
		ContentNS: "http://purl.org/rss/1.0/modules/content/",
		WPNS:      "http://wordpress.org/export/1.2/",
		Channel: wxrChannel{
			Title: "Budgie Blog Seed Data",
			Items: items,
		},
	}

	var out bytes.Buffer
	out.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	enc := xml.NewEncoder(&out)
	enc.Indent("", "  ")
	if err := enc.Encode(rss); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
