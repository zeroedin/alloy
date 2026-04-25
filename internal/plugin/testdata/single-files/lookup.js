// Test fixture: filter that accepts additional arguments
// Usage in Liquid: {{ "ready" | lookup: site.data.statusLegend }}
export default function(alloy) {
    alloy.filter("lookup", function(key, hash) {
        if (hash && typeof hash === 'object') {
            return hash[key] || key;
        }
        return key;
    });

    alloy.filter("replace_custom", function(input, search, replacement) {
        return input.split(search).join(replacement);
    });
}
