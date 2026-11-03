# Markdown page

This page body is the `markdown` attribute of a `notion_page`, written in Notion's enhanced markdown. The provider sends it once and doesn't read it back, because Notion normalizes markdown.

## Lists

- A bulleted item
- Another bulleted item

1. A numbered item
2. Another numbered item

- [ ] An open to-do
- [x] A finished to-do

## Quote and callout

> Documents can be managed like infrastructure.

<callout icon="💡">
	Callouts work in markdown too.
</callout>

## Code

```bash
terraform apply -auto-approve
terraform plan -detailed-exitcode
```

## Table

<table header-row="true">
	<tr>
		<td>Feature</td>
		<td>Where it is shown</td>
	</tr>
	<tr>
		<td>Blocks</td>
		<td>The Blocks page</td>
	</tr>
	<tr>
		<td>Databases</td>
		<td>The root page</td>
	</tr>
</table>

## Columns

Column layouts come from markdown here. The notion_block resource can't create them, because Notion only accepts a column list together with its columns and their content.

<columns>
	<column>
		Left column
	</column>
	<column>
		Right column
	</column>
</columns>

## Toggle

<details>
<summary>A toggle written in markdown</summary>
	Its content is hidden until it's opened.
</details>

## Math

$$
e^{i\pi} + 1 = 0
$$

---
