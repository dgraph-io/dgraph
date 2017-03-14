import React from "react";
import { ProgressBar } from "react-bootstrap";

function Progress(props) {
    let { display, perc } = props;
    return display &&
        <ProgressBar
            style={{ top: "50%", margin: "auto 10%", position: "relative" }}
            active={true}
            now={perc}
            min={0}
            max={100}
            label={`${perc}%`}
        />;
}

export default Progress;
