# Markdown page (v2)

This page body is sent with `markdown_version = "v2"`, so Notion reads it as Notion-flavored Markdown. Blocks are separated by blank lines, and blocks with children use their tag form.

## Lists

- A bulleted item
- Another bulleted item

1. A numbered item
2. Another numbered item

- [ ] An open to-do
- [x] A finished to-do

## Colored text

Some words are <span color="orange">orange</span> and some are <span color="blue_bg">on a blue background</span>.

## Callout and quote

<callout icon="🧭" color="yellow_bg">
A callout title written in v2

Its child paragraph comes after a blank line.
</callout>

<quote color="purple_bg">
A quote on a purple background
</quote>

## Toggle

<toggle>
A toggle written with the v2 tag

Its content is hidden until it's opened.
</toggle>

## Columns

<columns>
<column ratio="60">The wider left column</column>
<column ratio="40">The narrower right column</column>
</columns>

## Code

```bash
terraform apply -auto-approve
terraform plan -detailed-exitcode
```

## Math

$$
e^{i\pi} + 1 = 0
$$
