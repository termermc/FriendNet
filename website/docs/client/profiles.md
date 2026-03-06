# Profiles

You can create a profile for yourself on a server that other users can look at with HTML and CSS.

To create a profile for a server, create a share called `_profile` and put an `index.html` file
inside. Now you can add whatever HTML you want to it, including CSS, images and media.

## Limitations

For security reasons, scripts and a few interactive elements are not permitted inside profiles.

External resources of any kind are not allowed. You can reference resources by their path
relative to the share, like `css/styles.css`. Absolute paths like `/css/styles.css` are not
allowed.

Links (`<a>`) are allowed, but they must point to external websites and have the `target="_blank"`
attribute set. Users can right-click them to get the URL or middle click to open in a new tab,
but they will not be clickable in the normal way.

Despite these limitations, creating highly detailed profiles with complex CSS and media is very
easy.
