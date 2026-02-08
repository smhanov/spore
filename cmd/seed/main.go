package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	blog "github.com/smhanov/go-blog"
	"github.com/yuin/goldmark"
)

var budgiePosts = []struct {
	Title    string
	Markdown string
}{
	{
		Title: "The Ultimate Guide to Budgie Diet",
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
		Markdown: `Budgies are intelligent birds that need stimulation.

1. **Mirrors**: Budgies love interacting with their reflection (but use in moderation).
2. **Bells**: The noise is very satisfying for them.
3. **Swings**: A classic for a reason.
4. **Shredding Toys**: Great for their beak health.
5. **Puzzle Toys**: Hide treats inside to keep them busy.`,
	},
	{
		Title: "How to Teach Your Budgie to Talk",
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
		Markdown: `Budgies come in a rainbow of colors.

- **Green Series**: The wild type, dominant gene.
- **Blue Series**: A common recessive mutation.
- **Lutino**: Pure yellow with red eyes.
- **Albino**: Pure white with red eyes.

Genetics can be complex but fascinating!`,
	},
	{
		Title: "Choosing the Right Cage Size",
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
		Markdown: `Knowing what to look for can save your bird's life.

- **Bright Eyes**: Clear and alert.
- **Smooth Feathers**: Clean and well-preened.
- **Active Behavior**: Chirping and playing.
- **Clean Vent**: No staining around the tail.

If you notice puffing up or lethargy, see a vet immediately.`,
	},
	{
		Title: "Bonding with Your New Budgie",
		Markdown: `Building trust takes time.

1. **Give them space** for the first few days.
2. **Talk gently** to them through the cage.
3. **Offer treats** like millet through the bars.
4. **Hand taming**: Slowly introduce your hand inside the cage.

Consistency builds the strongest bond.`,
	},
	{
		Title: "Deciphering Budgie Sounds",
		Markdown: `What is your bird trying to say?

- **Chirping**: Happy and content.
- **Squawking**: Annoyed or demanding attention.
- **Beak Grinding**: A sign of relaxation, usually before sleep.
- **Hissing**: Threatened or aggressive.`,
	},
	{
		Title: "How Long Do Budgies Live?",
		Markdown: `In captivity, budgies typically live between 5 to 10 years, though some reach 15!

## Factors Influencing Longevity
- **Diet**: High quality nutrition extends life.
- **Exercise**: Flight time is essential.
- **Genetics**: Breeding plays a role.
- **Veterinary Care**: Regular checkups help preventing issues.`,
	},
	{
		Title: "Budgie Sleep Requirements",
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
	ctx := context.Background()

	fmt.Println("Seeding posts...")
	for i, bp := range budgiePosts {
		now := time.Now().Add(time.Duration(-i) * 24 * time.Hour) // Publish spaced out by days

		var buf bytes.Buffer
		if err := goldmark.Convert([]byte(bp.Markdown), &buf); err != nil {
			log.Fatalf("Failed to convert markdown for '%s': %v", bp.Title, err)
		}

		post := &blog.Post{
			ID:              uuid.New().String(),
			Slug:            fmt.Sprintf("budgie-post-%d", i+1),
			Title:           bp.Title,
			ContentMarkdown: bp.Markdown,
			ContentHTML:     buf.String(),
			PublishedAt:     &now,
			MetaDescription: fmt.Sprintf("Read about %s", bp.Title),
			AuthorID:        1,
		}

		if err := store.CreatePost(ctx, post); err != nil {
			log.Printf("Failed to create post '%s': %v", post.Title, err)
		} else {
			fmt.Printf("Created post: %s\n", post.Title)
		}
	}

	fmt.Println("Done!")
}
