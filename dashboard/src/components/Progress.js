import React from "react";
import { ProgressBar } from "react-bootstrap";

import "../assets/css/Graph.css";

function Progress({ perc }) {
    return <ProgressBar
        className="Graph-progress"
        active={true}
        now={perc}
        min={0}
        max={100}
        label={`${perc}%`}
    />
}

export default Progress;
