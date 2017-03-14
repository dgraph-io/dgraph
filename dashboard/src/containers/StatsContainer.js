import { connect } from "react-redux";

import Stats from "../components/Stats";

function renderTime(rendering) {
    if (rendering.end === undefined || rendering.start === undefined) {
        return "";
    }
    let timeTaken = (rendering.end.getTime() - rendering.start.getTime()) /
        1000;

    if (timeTaken > 1) {
        return timeTaken.toFixed(1) + "s";
    }
    return (timeTaken - Math.floor(timeTaken)) * 1000 + "ms";
}

const mapStateToProps = (state, ownProps) => ({
    server: state.latency.server,
    rendering: renderTime(state.latency.rendering)
});

export default connect(mapStateToProps, null)(Stats);
