# People

Use a person's page to see what an actor or director has worked on, and to find movies you want to ask for or add.

## Find a person

1. Open a movie or show page.
2. In the cast, press an actor or director.
3. Their page opens at `http://your-server:6262/people/<id>`.

People with the same name stay separate. Each page belongs to one person in the movie database.

## What the page shows

- **At a glance.** What they are known for, when and where they were born, and when they died.
- **Known for.** Their best known work.
- **Filmography.** Everything they are credited on, grouped by decade. Movies not out yet sit under **Coming soon**, and you can follow them from their card.
- **Often works with.** People they share credits with most.
- **About.** Their story, credits by decade and **Awards** (won and nominated). **Read more on Wikipedia** opens the full article.

## Filter and sort the filmography

- **Credit role** picks **All**, **Actor** or **Director**.
- **Sort** picks **Popular**, **Newest first** or **Oldest first**.

Each card says where the title stands for you: for example **In your Library**, **Requested** or **Approved, waiting for a copy**. Press a card to open the title, then ask for it or add it there.

Looking at a person adds nothing. HolyArrs does not follow a person's future work for you.

## Where the data comes from

People, credits and pictures come from the [metadata server](../metadata/metadata-service.md). Awards, birth details and the story come from Wikidata and Wikipedia through the same server. HolyArrs does not guess missing names or roles.

## Troubleshooting

- **No movies match this role.** The person has no credits in the role you picked. Switch **Credit role** to **All**.
- **No photo or story.** The metadata server has none for this person.
- **The page does not load.** HolyArrs cannot reach the metadata server. Try again later.
