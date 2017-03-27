import randomColor from "randomcolor";
import uuid from "uuid";
import _ from "lodash/object";

export function checkStatus(response) {
  if (response.status >= 200 && response.status < 300) {
    return response;
  } else {
    let error = new Error(response.statusText);
    error["response"] = response;
    throw error;
  }
}

export function timeout(ms, promise) {
  return new Promise(function(resolve, reject) {
    setTimeout(
      function() {
        reject(new Error("timeout"));
      },
      ms
    );
    promise.then(resolve, reject);
  });
}

export function aggregationPrefix(properties) {
  let aggTerms = ["max(", "min(", "sum("];
  for (let k in properties) {
    if (!properties.hasOwnProperty(k)) {
      continue;
    }
    if (k === "count") {
      return ["count", "count"];
    }
    for (let term of aggTerms) {
      if (k.startsWith(term)) {
        return [term.substr(0, term.length - 1), k];
      }
    }
  }
  return ["", ""];
}

function getNameKey(properties, regex) {
  for (let i in properties) {
    if (!properties.hasOwnProperty(i)) {
      continue;
    }
    if (regex.test(i)) {
      return i;
    }
  }
  return "";
}

function shortenName(label) {
  let words = label.split(" "), firstWord = words[0];
  if (firstWord.length > 20) {
    label = [firstWord.substr(0, 9), firstWord.substr(9, 7) + "..."].join(
      "-\n"
    );
  } else if (firstWord.length > 10) {
    label = [firstWord.substr(0, 9), firstWord.substr(9)].join("-\n");
  } else {
    // First word is less than 10 chars so we can display it in full.
    if (words.length > 1) {
      if (words[1].length > 10) {
        label = [firstWord, words[1] + "..."].join("\n");
      } else {
        label = [firstWord, words[1]].join("\n");
      }
    } else {
      label = firstWord;
    }
  }

  return label;
}

function getNodeLabel(properties: Object, regex: string): string {
  var label = "";

  let keys = Object.keys(properties);
  if (keys.length === 1) {
    label = aggregationPrefix(properties)[0];
    if (label !== "") {
      return label;
    }
  }

  let nameKey = getNameKey(properties, regex);
  return properties[nameKey] || "";
}

export function outgoingEdges(nodeId, edgeSet) {
  return edgeSet.get({
    filter: function(edge) {
      return edge.from === nodeId;
    }
  });
}

export function isShortestPath(query) {
  return query.indexOf("shortest") !== -1 &&
    query.indexOf("to") !== -1 &&
    query.indexOf("from") !== -1;
}

export function showTreeView(query) {
  return query.indexOf("orderasc") !== -1 || query.indexOf("orderdesc") !== -1;
}

export function isNotEmpty(response) {
  let keys = Object.keys(response);
  if (keys.length === 0) {
    return false;
  }

  for (let i = 0; i < keys.length; i++) {
    if (keys[i] !== "server_latency" && keys[i] !== "uids") {
      return keys[i].length > 0;
    }
  }
  return false;
}

function hasChildren(node: Object): boolean {
  for (var prop in node) {
    if (Array.isArray(node[prop])) {
      return true;
    }
  }
  return false;
}

function getRandomColor(randomColors) {
  if (randomColors.length === 0) {
    return randomColor();
  }

  let color = randomColors[0];
  randomColors.splice(0, 1);
  return color;
}

// TODO - Needs some refactoring. Too many arguments are passed.
function checkAndAssign(groups, pred, l, edgeLabels, randomColors) {
  // This label hasn't been allocated yet.
  groups[pred] = {
    label: l,
    color: getRandomColor(randomColors)
  };
  edgeLabels[l] = true;
}

// This function shortens and calculates the label for a predicate.
function getGroupProperties(
  pred: string,
  edgeLabels: MapOfBooleans,
  groups: GroupMap,
  randomColors
): Group {
  var prop = groups[pred];
  if (prop !== undefined) {
    // We have already calculated the label for this predicate.
    return prop;
  }

  let l;
  let dotIdx = pred.indexOf(".");
  if (dotIdx !== -1 && dotIdx !== 0 && dotIdx !== pred.length - 1) {
    l = pred[0] + pred[dotIdx + 1];
    checkAndAssign(groups, pred, l, edgeLabels, randomColors);
    return groups[pred];
  }

  for (var i = 1; i <= pred.length; i++) {
    l = pred.substr(0, i);
    // If the first character is not an alphabet we just continue.
    // This saves us from selecting ~ in case of reverse indexed preds.
    if (l.length === 1 && l.toLowerCase() === l.toUpperCase()) {
      continue;
    }
    if (edgeLabels[l] === undefined) {
      checkAndAssign(groups, pred, l, edgeLabels, randomColors);
      return groups[pred];
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }

  groups[pred] = {
    label: pred,
    color: getRandomColor(randomColors)
  };
  edgeLabels[pred] = true;
  return groups[pred];
}

function createAxisPlot(groups) {
  let axisPlot = [];
  for (let pred in groups) {
    if (!groups.hasOwnProperty(pred)) {
      continue;
    }

    axisPlot.push({
      label: groups[pred]["label"],
      pred: pred,
      color: groups[pred]["color"]
    });
  }

  return axisPlot;
}

function extractFacets(val, edgeAttributes, properties) {
  // lets handle @facets between uids here.
  for (let pred in val) {
    if (!val.hasOwnProperty(pred)) {
      continue;
    }

    // pred could either be _ or other predicates. If its a predicate it could have
    // multiple k-v pairs.
    if (pred === "_") {
      edgeAttributes["facets"] = val["_"];
    } else {
      let predFacets = val[pred];
      for (let f in predFacets) {
        if (!predFacets.hasOwnProperty(f)) {
          continue;
        }

        properties["facets"][`${pred}[${f}]`] = predFacets[f];
      }
    }
  }
}

function findAndMerge(nodes, n) {
  let properties = JSON.parse(n.title),
    uid = properties["attrs"]["_uid_"],
    idx = nodes.findIndex(function(node) {
      return node.id === uid;
    });

  if (idx === -1) {
    console.warn("Expected to find node with uid: ", uid);
    return;
  }

  let node = nodes[idx], props = JSON.parse(node.title);
  _.merge(props, properties);
  node.title = JSON.stringify(props);
  // For shortest path, this would overwrite the color and this is fine
  // because actual shortes path is traversed later.
  node.color = n.color;

  if (node.label === "") {
    node.label = n.label;
  }
  if (node.name === "" && n.name !== "") {
    node.name = n.name;
  }
  nodes[idx] = node;
}

export function processGraph(
  response: Object,
  treeView: boolean,
  query: string,
  regex: string
) {
  let nodesQueue: Array<ResponseNode> = [],
    // Contains map of a lable to its shortform thats displayed.
    predLabel: MapOfStrings = {},
    // Map of whether a Node with an Uid has already been created. This helps
    // us avoid creating duplicating nodes while parsing the JSON structure
    // which is a tree.
    uidMap: MapOfBooleans = {},
    edgeMap: MapOfBooleans = {},
    nodes: Array<Node> = [],
    edges: Array<Edge> = [],
    emptyNode: ResponseNode = {
      node: {},
      src: {
        id: "",
        pred: "empty"
      }
    },
    someNodeHasChildren: boolean = false,
    ignoredChildren: Array<ResponseNode> = [],
    // We store the indexes corresponding to what we show at first render here.
    // That we can only do one traversal.
    nodesIndex,
    edgesIndex,
    // level = 0,
    // Picked up from http://graphicdesign.stackexchange.com/questions/3682/where-can-i-find-a-large-palette-set-of-contrasting-colors-for-coloring-many-d
    randomColorList = [
      "#47c0ee",
      "#8dd593",
      "#f6c4e1",
      "#8595e1",
      "#f0b98d",
      "#f79cd4",
      "#bec1d4",
      "#11c638",
      "#b5bbe3",
      "#7d87b9",
      "#e07b91",
      "#4a6fe3"
    ],
    // Stores the map of a label to boolean (only true values are stored).
    // This helps quickly find if a label has already been assigned.
    groups = {},
    displayLabel,
    fullName,
    regexEx = new RegExp(regex);

  for (var k in response) {
    if (!response.hasOwnProperty(k)) {
      return;
    }

    someNodeHasChildren = false;
    ignoredChildren = [];

    if (k !== "server_latency" && k !== "uids") {
      let block = response[k];
      for (let i = 0; i < block.length; i++) {
        let rn: ResponseNode = {
          node: block[i],
          src: {
            id: "",
            pred: k
          }
        };

        if (hasChildren(block[i])) {
          someNodeHasChildren = true;
          nodesQueue.push(rn);
        } else {
          ignoredChildren.push(rn);
        }
      }

      // If no root node has children , then we add all root level nodes to the view.
      if (!someNodeHasChildren) {
        nodesQueue.push.apply(nodesQueue, ignoredChildren);
      }
    }
  }

  // We push an empty node after all the children. This would help us know when
  // we have traversed all nodes at a level.
  nodesQueue.push(emptyNode);

  while (nodesQueue.length > 0) {
    let obj = nodesQueue.shift();

    // Check if this is an empty node.
    if (Object.keys(obj.node).length === 0 && obj.src.pred === "empty") {
      // Only nodes and edges upto nodesIndex, edgesIndex are rendered on first load.
      if (nodesIndex === undefined && nodes.length > 50) {
        // Nodes upto level 1 are rendered only if the total number is less than 100. Else only
        // nodes at level 0 are rendered.
        nodesIndex = nodes.length;
        edgesIndex = edges.length;
      }

      // If no more nodes left, then we can break.
      if (nodesQueue.length === 0) {
        break;
      }

      nodesQueue.push(emptyNode);
      // level++;
      continue;
    }

    let properties: MapOfStrings = {
      attrs: {},
      facets: {}
    },
      id: string,
      edgeAttributes = {
        facets: {}
      },
      uid: string;

    // Some nodes like results of aggregation queries, max , min, count etc don't have a
    // _uid_, so we need to assign thme one.
    uid = obj.node["_uid_"] === undefined ? uuid() : obj.node["_uid_"];
    id = treeView
      ? // For tree view, the id is the join of ids of this node
        // with all its ancestors. That would make it unique.
        [obj.src.id, uid].filter(val => val).join("-")
      : uid;

    for (let prop in obj.node) {
      if (!obj.node.hasOwnProperty(prop)) {
        continue;
      }

      // We can have a key-val pair, another array or an object here (in case of facets)
      let val = obj.node[prop];
      if (Array.isArray(val)) {
        // These are child nodes, lets add them to the queue.
        let arr = val, xposition = 1;
        for (let j = 0; j < arr.length; j++) {
          // X position makes sure that nodes are rendered in the order they are received
          // in the response.
          arr[j]["x"] = xposition++;
          nodesQueue.push({
            node: arr[j],
            src: {
              pred: prop,
              id: id
            }
          });
        }
      } else if (typeof val === "object") {
        if (prop === "@facets") {
          extractFacets(val, edgeAttributes, properties);
        }
      } else {
        properties["attrs"][prop] = val;
      }
    }

    let nodeAttrs = properties["attrs"],
      // aggrTerm can be count, min or max. aggrPred is the actual predicate returned.
      [aggrTerm, aggrPred] = aggregationPrefix(nodeAttrs),
      name = aggrTerm !== "" ? aggrTerm : obj.src.pred,
      props = getGroupProperties(name, predLabel, groups, randomColorList),
      x = nodeAttrs["x"];

    delete nodeAttrs["x"];

    if (aggrTerm !== "") {
      displayLabel = nodeAttrs[aggrPred];
    } else {
      fullName = regex === "" ? "" : getNodeLabel(nodeAttrs, regexEx);
      displayLabel = shortenName(fullName);
    }

    let n: Node = {
      id: id,
      uid: obj.node["_uid_"],
      x: x,
      // For aggregation nodes, label is the actual value, for other nodes its
      // the value of name.
      label: displayLabel,
      title: JSON.stringify(properties),
      color: props.color,
      group: obj.src.pred,
      name: fullName
    };

    if (uidMap[id] === undefined) {
      // For tree view, we can't push duplicates because two query blocks might have the
      // same root node and child elements won't really have the same uids as their uid is a
      // combination of all their ancestor uids.
      uidMap[id] = true;
      nodes.push(n);
    } else {
      // We have already put this node. So we need to find the node in nodes,
      // merge new properties and put it back.
      findAndMerge(nodes, n);
    }

    // Render only first 1000 nodes on first load otherwise graph can get stuck.
    if (nodes.length > 1000 && nodesIndex === undefined) {
      nodesIndex = nodes.length;
      edgesIndex = edges.length;
    }

    // Root nodes don't have a source node, so we don't want to create any edge for them.
    if (obj.src.id === "") {
      continue;
    }

    let fromTo = [obj.src.id, id].filter(val => val).join("-");

    if (edgeMap[fromTo]) {
      let edgeIdx = edges.findIndex(function(edge) {
        return edge.from === obj.src.id && edge.to === id;
      });
      if (edgeIdx === -1) {
        continue;
      }

      let oldEdge = edges[edgeIdx], edgeTitle = JSON.parse(oldEdge.title);
      // This is helpful in case of shortest path results so that we can get
      // the edge weights.
      _.merge(edgeAttributes, edgeTitle);
      oldEdge.title = JSON.stringify(edgeAttributes);
      edges[edgeIdx] = oldEdge;
    } else {
      edgeMap[fromTo] = true;

      var e: Edge = {
        from: obj.src.id,
        to: id,
        title: JSON.stringify(edgeAttributes),
        label: props.label,
        color: props.color,
        arrows: "to"
      };
      edges.push(e);
    }
  }

  return [nodes, edges, createAxisPlot(groups), nodesIndex, edgesIndex];
}

export function sortStrings(a, b) {
  var nameA = a.toLowerCase(), nameB = b.toLowerCase();
  if (
    nameA < nameB //sort string ascending
  )
    return -1;
  if (nameA > nameB) return 1;
  return 0; //default return value (no sorting)
}

export function dgraphAddress() {
  if (process.env.NODE_ENV === "production") {
    // This is defined in index.html and we get it from the url.
    return window.SERVER_URL;
  }

  // For development, we just connect to the Dgraph server at http://localhost:8080.
  return "http://localhost:8080";
}
