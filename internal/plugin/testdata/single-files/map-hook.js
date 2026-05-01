export default function(alloy) {
  alloy.hook('onContentTransformed', (page) => {
    page.html = page.html + '<!-- modified -->';
    page.frontMatter.injected = 'by-plugin';
    return page;
  });
}
