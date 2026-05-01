export default function(alloy) {
  alloy.hook('onContentTransformed', (page) => {
    page.frontMatter.nested.added = true;
    return page;
  });
}
