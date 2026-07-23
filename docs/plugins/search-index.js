export const runtime = "node";

export default function(alloy) {
  let searchEntries = [];

  // Register output path for conflict detection
  alloy.hook("onBeforeValidation", {}, () => {
    return {
      addOutputs: {
        "search-index.json": "plugin:search-index"
      }
    };
  });

  // Collect rendered page content
  alloy.hook("onContentLoaded", {
    pages: true,
    pageFields: ["frontMatter", "html", "url"]
  }, (pages) => {
    searchEntries = [];
    for (const page of pages) {
      if (page.frontMatter.layout !== "doc") continue;

      // Strip HTML tags to get plain text body
      const body = (page.html || "")
        .replace(/<[^>]+>/g, " ")
        .replace(/&[a-zA-Z]+;/g, " ")
        .replace(/&#\d+;/g, " ")
        .replace(/\s+/g, " ")
        .trim();

      searchEntries.push({
        title: page.frontMatter.title || "",
        link: page.url,
        section: page.frontMatter.nav_section || "",
        description: page.frontMatter.description || "",
        body: body,
      });
    }
    searchEntries.sort((a, b) =>
      a.section.localeCompare(b.section) || a.title.localeCompare(b.title)
    );
    return pages;
  });

  // Write the index after the build finishes
  // TODO: use result.OutputDir once #1111 is resolved
  alloy.hook("onBuildComplete", {}, async () => {
    const { writeFileSync } = await import("fs");
    const { join } = await import("path");
    writeFileSync(
      join("_site", "search-index.json"),
      JSON.stringify(searchEntries),
    );
  });
}
