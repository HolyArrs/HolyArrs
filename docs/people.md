# People and movie credits

The People screen lets you find an actor or director and browse the movies credited to that exact provider identity. People with the same name stay separate. The search result shows the provider id indirectly through its link, the person's usual department and up to three known movies so you can choose the right person.

HolyArrs currently reads people and movie credits from TMDB when that movie metadata connection is available. Person search uses TMDB's pages. A person's movie credits come from TMDB's single, unpaged movie-credit response. HolyArrs labels that filmography as incomplete because the provider does not promise that the response is a complete historical record.

On a person's page you can filter credits to Actor or Director. A movie credited more than once appears once. Its card retains every distinct acting character and whether the person was also the director. Crew credits count as directing only when the provider marks the job as Director.

Each movie is joined to HolyArrs by its stable TMDB movie id. The page shows whether it is already in the Library, on Never-add, part of an open Wishlist ask or missing. These checks run again when you review a selection.

Select missing movies, then choose **Review selected**. The review names the exact movies that will be submitted. People who manage the Library add each reviewed movie through the normal one-movie add path. People with Wishlist permission send each one through the normal ask path, including the existing rolling weekly limit. One refusal does not hide the result for the other movies.

Browsing and filtering do not add anything. HolyArrs does not follow a person's future credits and does not subscribe to their whole filmography. You must select and review movies each time.

Movie detail pages can link each returned actor and director to the same person page. Profile and poster images are shown only when the configured metadata provider returns them through its normal image contract.

If the metadata provider has no people support, its connection is unavailable or it returns an unusable answer, the page says that people results are unavailable. It does not replace missing credits with guessed names or roles.
