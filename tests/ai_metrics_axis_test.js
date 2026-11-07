const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");

const app = fs.readFileSync("internal/ui/static/js/app.js", "utf8");
const start = app.indexOf("function calculateAIMetricsYAxis");
const end = app.indexOf("\nfunction renderAIMetricsChart", start);
const calculateAIMetricsYAxis = Function(
    `${app.slice(start, end)}; return calculateAIMetricsYAxis;`,
)();
const valueCheckStart = app.indexOf("function hasAIMetricsChartValue");
const valueCheckEnd = app.indexOf("\nfunction aiMetricsChartSeries", valueCheckStart);
const hasAIMetricsChartValue = Function(
    `${app.slice(valueCheckStart, valueCheckEnd)}; return hasAIMetricsChartValue;`,
)();
const seriesStart = app.indexOf("function aiMetricsChartSeries");
const seriesEnd = app.indexOf("\nfunction renderAIMetricsChart", seriesStart);
const aiMetricsChartSeries = Function(
    `${app.slice(seriesStart, seriesEnd)}; return aiMetricsChartSeries;`,
)();
const pointValue = (point, key) => point[key] ?? null;

test("detects empty discovery chart data", () => {
    assert.equal(hasAIMetricsChartValue([{ date: "2026-08-03", rate: null }]), false);
    assert.equal(hasAIMetricsChartValue([{ date: "2026-08-03", rate: 0 }]), true);
});

test("selects the discovery rate chart series", () => {
    const series = aiMetricsChartSeries({ dataset: { isDiscovery: "true" } });

    assert.deepEqual(series.map((item) => item.key), ["rate", "classifier_upvote_rate", "recall"]);
    assert.equal(series[1].color, "#ea580c");
    assert.equal(series[1].label, "Classifier upvote rate");
    series[1].visible = false;
    assert.equal(series[0].visible, true);
});

test("selects the Relevance ranking chart series", () => {
    const series = aiMetricsChartSeries({ dataset: { isRelevance: "true" } });

    assert.deepEqual(series.map((item) => item.key), [
        "average_precision",
        "precision_at_50",
    ]);
});

test("zooms metrics near their data", () => {
    const axis = calculateAIMetricsYAxis(
        [{ f1: 0.90 }, { f1: 0.95 }],
        [{ key: "f1" }],
        pointValue,
    );

    assert.deepEqual(axis, {
        minimum: 0.8,
        maximum: 1,
        ticks: [0.8, 0.85, 0.9, 0.95, 1],
    });
});

test("uses the full valid range when no visible value exists", () => {
    assert.deepEqual(calculateAIMetricsYAxis([], [], pointValue), {
        minimum: 0,
        maximum: 1,
        ticks: [0, 0.25, 0.5, 0.75, 1],
    });
});

test("uses the classifier label for its tooltip series", () => {
    const series = aiMetricsChartSeries({ dataset: {
        isDiscovery: "true",
        labelClassifierUpvoteRate: "Translated classifier rate",
    } });
    assert.equal(series[1].label, "Translated classifier rate");
});

test("keeps missing classifier rates separate from zero", () => {
    assert.equal(hasAIMetricsChartValue([{ rate: null, classifier_upvote_rate: null }]), false);
    assert.equal(hasAIMetricsChartValue([{ rate: null, classifier_upvote_rate: 0 }]), true);
    const axis = calculateAIMetricsYAxis(
        [{ rate: 0.9, classifier_upvote_rate: null }, { rate: 0.95, classifier_upvote_rate: 0.5 }],
        aiMetricsChartSeries({ dataset: { isDiscovery: "true" } }),
        pointValue,
    );
    assert.ok(axis.minimum <= 0.5);
    assert.ok(axis.maximum >= 0.95);
});

test("keeps recall separate from the other discovery series", () => {
    const series = aiMetricsChartSeries({ dataset: { isDiscovery: "true", labelRecall: "Translated recall" } });
    assert.equal(series[2].key, "recall");
    assert.equal(series[2].color, "#2563eb");
    assert.equal(series[2].label, "Translated recall");
    series[2].visible = false;
    assert.equal(series[0].visible, true);
    assert.equal(series[1].visible, true);
    assert.equal(hasAIMetricsChartValue([{date: "2026-08-03", recall: 0}]), true);
    assert.equal(hasAIMetricsChartValue([{date: "2026-08-03", recall: null}]), false);
});
