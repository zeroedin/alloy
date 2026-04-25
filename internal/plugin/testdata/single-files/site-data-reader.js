// Test fixture: plugin that reads alloy.data
export default function(alloy) {
    alloy.filter("statusPretty", function(key) {
        return alloy.data.statusLegend[key].pretty;
    });

    alloy.filter("navCount", function() {
        return alloy.data.nav.length;
    });
}
