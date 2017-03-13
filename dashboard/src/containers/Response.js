import React, { Component } from "react";

import GraphContainer from "./GraphContainer";
import ResponseInfoContainer from "./ResponseInfoContainer";

import "../assets/css/Response.css";

class Response extends Component {
    constructor(props: Props) {
        super(props);
        this.expand = this.expand.bind(this);
    }

    expand = () => {
        // TODO - Maybe check how can we get rid of this ref stuff.
        this.refs.graph.getWrappedInstance().expand();
    };

    render() {
        return (
            <div className="Response" id="response">
                <GraphContainer ref="graph" />
                <ResponseInfoContainer expand={this.expand} />
            </div>
        );
    }
}

export default Response;
