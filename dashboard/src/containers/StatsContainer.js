import { connect } from "react-redux";

import Stats from "../components/Stats";

function renderTime(rendering) {
    if (rendering.end === undefined || rendering.start === undefined) {
        return "";
    }

    if (rendering.end.getTime() < rendering.start.getTime()) {
        return;
    }
    let timeTaken = (rendering.end.getTime() - rendering.start.getTime()) /
        1000;

    if (timeTaken > 1) {
        return timeTaken.toFixed(1) + "s";
    }
    return ((timeTaken - Math.floor(timeTaken)) * 1000).toFixed(0) + "ms";
}

const mapStateToProps = (state, ownProps) => ({
    server: state.latency.server,
    rendering: renderTime(state.latency.rendering)
});

export default connect(mapStateToProps, null)(Stats);
