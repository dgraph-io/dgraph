import React, { Component } from "react";
import { Navbar, Nav, NavItem } from "react-bootstrap";

import logo from "../assets/images/logo.svg";

class NavBar extends Component {
    render() {
        return (
            <Navbar style={{ borderBottom: "0.5px solid gray" }} fluid={true}>
                <Navbar.Header>
                    <Navbar.Brand>
                        <a href="#" style={{ paddingTop: "10px" }}>
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
                    </Nav>
                </Navbar.Collapse>
            </Navbar>
        );
    }
}

export default NavBar;
