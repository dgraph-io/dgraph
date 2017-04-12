import React from "react";
import { Navbar, Nav, NavItem, Button } from "react-bootstrap";
import CopyToClipboard from "react-copy-to-clipboard";

import { dgraphAddress } from "../containers/Helpers.js";

import logo from "../assets/images/logo.svg";
import "../assets/css/Navbar.css";

function url(id) {
    return dgraphAddress() + "/" + id;
}

const NavBar = React.createClass({
    getInitialState() {
        return {
            shareUrl: ""
        };
    },

    componentWillReceiveProps(nextProps) {
        if (
            nextProps.shareId !== "" && nextProps.shareId !== this.props.shareId
        ) {
            this.setState({ shareUrl: url(nextProps.shareId) });
        }
    },

    render() {
        let { getShareId, shareId } = this.props,
            urlClass = shareId === "" ? "Nav-url-hide" : "";
        return (
            <Navbar style={{ borderBottom: "0.5px solid gray" }} fluid={true}>
                <Navbar.Header>
                    <Navbar.Brand>
                        <a
                            href="https://dgraph.io"
                            target="blank"
                            style={{ paddingTop: "10px" }}
                        >
                            <img src={logo} width="100" height="30" alt="" />
                        </a>
                    </Navbar.Brand>
                    <Navbar.Toggle />
                </Navbar.Header>
                <Navbar.Collapse>
                    <Nav>
                        <NavItem target="_blank" href="https://docs.dgraph.io">
                            Documentation
                        </NavItem>
                        <NavItem
                            target="_blank"
                            href="https://github.com/dgraph-io/dgraph"
                        >
                            Github
                        </NavItem>
                        <NavItem target="_blank" href="https://open.dgraph.io">
                            Blog
                        </NavItem>
                        <NavItem
                            target="_blank"
                            href="https://dgraph.slack.com"
                        >
                            Community
                        </NavItem>
                        <NavItem className="Nav-pad hidden-xs">
                            <form className="form-inline">
                                <button
                                    className="btn btn-default"
                                    onClick={e => {
                                        e.preventDefault();
                                        getShareId();
                                    }}
                                >
                                    Share
                                </button>
                                <input
                                    className={
                                        `form-control Nav-share-url ${urlClass}`
                                    }
                                    type="text"
                                    value={url(shareId)}
                                    onChange={() => {}}
                                    placeholder="Share"
                                />
                                <CopyToClipboard text={this.state.shareUrl}>
                                    <Button
                                        style={{ marginLeft: "5px" }}
                                        className={`${urlClass}`}
                                    >
                                        <i
                                            className={`fa fa-files-o`}
                                            aria-hidden="true"
                                        />
                                    </Button>
                                </CopyToClipboard>
                            </form>
                        </NavItem>
                    </Nav>
                </Navbar.Collapse>
            </Navbar>
        );
    }
});

export default NavBar;
