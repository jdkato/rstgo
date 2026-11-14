package rstgo

// Port of docutils.transforms.references.

import (
	"fmt"
	"strings"
)

var targetableTags = set("target footnote citation")

func xPropagateTargets(d *Document, _ *Node) {
	for _, target := range d.Root.findTag("target") {
		if target.Parent == nil {
			continue
		}
		if textElementTags[target.Parent.Tag] ||
			target.has("refid") || target.has("refuri") || target.has("refname") {
			continue
		}
		nextNode := target.nextNode(nil, true, false, true)
		for nextNode != nil && nextNode.Tag == "system_message" {
			nextNode = nextNode.nextNode(nil, false, false, true)
		}
		if nextNode == nil || nextNode.IsText() ||
			((invisibleTags[nextNode.Tag] || targetableTags[nextNode.Tag]) && nextNode.Tag != "target") {
			continue
		}
		nextNode.appendTo("ids", target.list("ids")...)
		nextNode.appendTo("names", target.list("names")...)
		if nextNode.expectByName == nil {
			nextNode.expectByName = map[string]*Node{}
		}
		if nextNode.expectByID == nil {
			nextNode.expectByID = map[string]*Node{}
		}
		for _, id := range target.list("ids") {
			d.ids[id] = nextNode
			nextNode.expectByID[id] = target
		}
		for _, name := range target.list("names") {
			nextNode.expectByName[name] = target
		}
		for k, v := range target.expectByName {
			nextNode.expectByName[k] = v
		}
		for k, v := range target.expectByID {
			nextNode.expectByID[k] = v
		}
		if target.Parent.Tag == "figure" && nextNode.Tag == "caption" {
			target.Parent.remove(target)
			continue
		}
		target.setAttr("refid", target.list("ids")[0])
		target.Attrs["ids"] = []string{}
		target.Attrs["names"] = []string{}
		d.noteRefid(target)
	}
}

func (d *Document) newProblematic(ref *Node, msg *Node) {
	msgid := d.setID(msg, nil, "")
	prb := newTextElem("problematic", ref.Rawsource, ref.Rawsource)
	prb.setAttr("refid", msgid)
	prbid := d.setID(prb, nil, "")
	msg.addBackref(prbid)
	ref.replaceSelf(prb)
}

func xAnonymousHyperlinks(d *Document, _ *Node) {
	var refs, targets []*Node
	for _, n := range d.Root.findTag("reference") {
		if n.truthy("anonymous") {
			refs = append(refs, n)
		}
	}
	for _, n := range d.Root.findTag("target") {
		if n.truthy("anonymous") {
			targets = append(targets, n)
		}
	}
	if len(refs) != len(targets) {
		msg := d.reporter.error(fmt.Sprintf("Anonymous hyperlink mismatch: %d references but %d "+
			"targets.\nSee \"backrefs\" attribute for IDs.", len(refs), len(targets)), nil)
		for _, ref := range refs {
			d.newProblematic(ref, msg)
		}
		return
	}
	for i, ref := range refs {
		target := targets[i]
		if ref.has("refid") || ref.has("refuri") {
			continue
		}
		target.referenced = true
		for guard := 0; guard < 10000; guard++ {
			if target.has("refuri") {
				ref.setAttr("refuri", target.get("refuri"))
				ref.resolved = true
				break
			}
			if len(target.list("ids")) == 0 {
				t2 := d.ids[target.str("refid")]
				if t2 == nil {
					break
				}
				target = t2
				continue
			}
			ref.setAttr("refid", target.list("ids")[0])
			d.noteRefid(ref)
			break
		}
	}
}

func xIndirectHyperlinks(d *Document, _ *Node) {
	multiply := map[*Node]bool{}
	var resolveTarget func(target *Node)
	var resolveRefs func(target *Node)
	targetError := func(target *Node, explanation string) {
		naming := ""
		var reflist []*Node
		names := target.list("names")
		if len(names) > 0 {
			naming = fmt.Sprintf(`"%s" `, names[0])
		}
		for _, name := range names {
			reflist = append(reflist, d.refnames[name]...)
		}
		for _, id := range target.list("ids") {
			reflist = append(reflist, d.refids[id]...)
		}
		if ids := target.list("ids"); len(ids) > 0 {
			naming += fmt.Sprintf(`(id="%s")`, ids[0])
		}
		msg := d.reporter.error(fmt.Sprintf(`Indirect hyperlink target %s refers to target "%s", %s.`,
			naming, target.str("refname"), explanation), nil, baseNode(target))
		msgid := d.setID(msg, nil, "")
		seen := map[*Node]bool{}
		for _, ref := range reflist {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			prb := newTextElem("problematic", ref.Rawsource, ref.Rawsource)
			prb.setAttr("refid", msgid)
			prbid := d.setID(prb, nil, "")
			msg.addBackref(prbid)
			if ref.Parent != nil {
				ref.replaceSelf(prb)
			}
		}
		target.resolved = true
	}
	nonexistent := func(target *Node) {
		if v, ok := d.names[target.str("refname")]; ok && v == nil {
			targetError(target, "which is a duplicate, and cannot be used as a unique reference")
		} else {
			targetError(target, "which does not exist")
		}
	}
	resolveTarget = func(target *Node) {
		refname, hasRefname := target.str("refname"), target.has("refname")
		refid := target.str("refid")
		var reftarget *Node
		if refid != "" {
			reftarget = d.ids[refid]
		} else {
			reftarget = d.names[refname]
			refid = d.nameids[refname]
			if reftarget != nil && refid == "" {
				refid = d.setID(reftarget, nil, "")
			}
		}
		if reftarget == nil {
			nonexistent(target)
			return
		}
		reftarget.noteReferencedBy("", refid)
		if reftarget.Tag == "target" && !reftarget.resolved && reftarget.has("refname") {
			if multiply[target] {
				targetError(target, "forming a circular reference")
				return
			}
			multiply[target] = true
			resolveTarget(reftarget)
			delete(multiply, target)
		}
		if reftarget.has("refuri") {
			target.setAttr("refuri", reftarget.get("refuri"))
			target.delAttr("refid")
		} else if reftarget.has("refid") {
			target.setAttr("refid", reftarget.get("refid"))
			d.noteRefid(target)
		} else {
			if len(reftarget.list("ids")) > 0 {
				target.setAttr("refid", refid)
				d.noteRefid(target)
			} else {
				nonexistent(target)
				return
			}
		}
		if hasRefname {
			target.delAttr("refname")
		}
		target.resolved = true
	}
	resolveRefs = func(target *Node) {
		var attname string
		if target.has("refid") {
			attname = "refid"
		} else if target.has("refuri") {
			attname = "refuri"
		} else {
			return
		}
		attval := target.get(attname)
		for _, name := range target.list("names") {
			reflist := d.refnames[name]
			if len(reflist) > 0 {
				target.noteReferencedBy(name, "")
			}
			for _, ref := range reflist {
				if ref.resolved {
					continue
				}
				ref.delAttr("refname")
				ref.setAttr(attname, attval)
				if attname == "refid" {
					d.noteRefid(ref)
				}
				ref.resolved = true
				if ref.Tag == "target" {
					resolveRefs(ref)
				}
			}
		}
		for _, id := range target.list("ids") {
			reflist := d.refids[id]
			if len(reflist) > 0 {
				target.noteReferencedBy("", id)
			}
			for _, ref := range append([]*Node{}, reflist...) {
				if ref.resolved {
					continue
				}
				ref.delAttr("refid")
				ref.setAttr(attname, attval)
				if attname == "refid" {
					d.noteRefid(ref)
				}
				ref.resolved = true
				if ref.Tag == "target" {
					resolveRefs(ref)
				}
			}
		}
	}
	for _, target := range d.indirectTargets {
		if !target.resolved {
			resolveTarget(target)
		}
		resolveRefs(target)
	}
}

func xExternalTargets(d *Document, _ *Node) {
	for _, target := range d.Root.findTag("target") {
		if target.has("refuri") {
			refuri := target.get("refuri")
			for _, name := range target.list("names") {
				reflist := d.refnames[name]
				if len(reflist) > 0 {
					target.noteReferencedBy(name, "")
				}
				for _, ref := range reflist {
					if ref.resolved {
						continue
					}
					ref.delAttr("refname")
					ref.setAttr("refuri", refuri)
					ref.resolved = true
				}
			}
		}
	}
}

func xInternalTargets(d *Document, _ *Node) {
	for _, target := range d.Root.findTag("target") {
		if !target.has("refuri") && !target.has("refid") {
			for _, name := range target.list("names") {
				refid := d.nameids[name]
				reflist := d.refnames[name]
				if len(reflist) > 0 {
					target.noteReferencedBy(name, "")
				}
				for _, ref := range reflist {
					if ref.resolved {
						continue
					}
					if refid != "" {
						ref.delAttr("refname")
						ref.setAttr("refid", refid)
					}
					ref.resolved = true
				}
			}
		}
	}
}

var footnoteSymbols = []string{"*", "†", "‡", "§", "¶", "#",
	"♠", "♥", "♦", "♣"}

func xFootnotes(d *Document, _ *Node) {
	var autofootnoteLabels []string
	startnum := d.autofootnoteStart
	origStart := startnum
	_ = origStart
	// number_footnotes
	for _, footnote := range d.autofootnotes {
		var label string
		for {
			label = fmt.Sprint(startnum)
			startnum++
			if _, ok := d.nameids[label]; !ok {
				break
			}
		}
		footnote.insert(0, newTextElem("label", "", label))
		for _, name := range footnote.list("names") {
			for _, ref := range d.footnoteRefs[name] {
				ref.append(newText(label))
				ref.delAttr("refname")
				ref.setAttr("refid", footnote.list("ids")[0])
				footnote.addBackref(ref.list("ids")[0])
				d.noteRefid(ref)
				ref.resolved = true
			}
		}
		if len(footnote.list("names")) == 0 && len(footnote.list("dupnames")) == 0 {
			footnote.appendTo("names", label)
			d.noteExplicitTarget(footnote, footnote)
			autofootnoteLabels = append(autofootnoteLabels, label)
		}
	}
	d.autofootnoteStart = startnum
	// number_footnote_references
	i := 0
	for _, ref := range d.autofootnoteRefs {
		if ref.resolved || ref.has("refid") {
			continue
		}
		if i >= len(autofootnoteLabels) {
			n := len(autofootnoteLabels)
			s := ""
			if n > 1 {
				s = "s"
			}
			msg := d.reporter.error(fmt.Sprintf("Too many autonumbered footnote references: "+
				"only %d corresponding footnote%s available.", n, s), nil, baseNode(ref))
			msgid := d.setID(msg, nil, "")
			for _, r := range d.autofootnoteRefs[i:] {
				if r.resolved || r.has("refname") {
					continue
				}
				prb := newTextElem("problematic", r.Rawsource, r.Rawsource)
				prb.setAttr("refid", msgid)
				prbid := d.setID(prb, nil, "")
				msg.addBackref(prbid)
				r.replaceSelf(prb)
			}
			break
		}
		label := autofootnoteLabels[i]
		ref.append(newText(label))
		id := d.nameids[label]
		footnote := d.ids[id]
		ref.setAttr("refid", id)
		d.noteRefid(ref)
		footnote.addBackref(ref.list("ids")[0])
		ref.resolved = true
		i++
	}
	// symbolize_footnotes
	var labels []string
	for _, footnote := range d.symbolFootnotes {
		reps, index := d.symbolFootStart/len(footnoteSymbols), d.symbolFootStart%len(footnoteSymbols)
		labeltext := strings.Repeat(footnoteSymbols[index], reps+1)
		labels = append(labels, labeltext)
		footnote.insert(0, newTextElem("label", "", labeltext))
		d.symbolFootStart++
		d.setID(footnote, nil, "")
	}
	i = 0
	for _, ref := range d.symbolFootnoteRef {
		if i >= len(labels) {
			msg := d.reporter.error(fmt.Sprintf("Too many symbol footnote references: only %d "+
				"corresponding footnotes available.", len(labels)), nil, baseNode(ref))
			msgid := d.setID(msg, nil, "")
			for _, r := range d.symbolFootnoteRef[i:] {
				if r.resolved || r.has("refid") {
					continue
				}
				prb := newTextElem("problematic", r.Rawsource, r.Rawsource)
				prb.setAttr("refid", msgid)
				prbid := d.setID(prb, nil, "")
				msg.addBackref(prbid)
				r.replaceSelf(prb)
			}
			break
		}
		ref.append(newText(labels[i]))
		footnote := d.symbolFootnotes[i]
		ref.setAttr("refid", footnote.list("ids")[0])
		d.noteRefid(ref)
		footnote.addBackref(ref.list("ids")[0])
		i++
	}
	// resolve_footnotes_and_citations
	resolve := func(note *Node, reflist []*Node) {
		id := note.list("ids")[0]
		for _, ref := range reflist {
			if ref.resolved {
				continue
			}
			ref.delAttr("refname")
			ref.setAttr("refid", id)
			note.addBackref(ref.list("ids")[0])
			ref.resolved = true
		}
		note.resolved = true
	}
	for _, footnote := range d.footnotes {
		for _, label := range footnote.list("names") {
			if reflist, ok := d.footnoteRefs[label]; ok {
				resolve(footnote, reflist)
			}
		}
	}
	for _, citation := range d.citations {
		for _, label := range citation.list("names") {
			if reflist, ok := d.citationRefs[label]; ok {
				resolve(citation, reflist)
			}
		}
	}
}

func xSubstitutions(d *Document, _ *Node) {
	defs := d.substitutionDefs
	normed := d.substitutionNames
	nested := map[string][]string{}
	origin := map[*Node]*Node{}
	subreflist := d.Root.findTag("substitution_reference")
	for idx := 0; idx < len(subreflist); idx++ {
		ref := subreflist[idx]
		var msg *Node
		refname := ref.str("refname")
		key, found := "", false
		if _, ok := defs[refname]; ok {
			key, found = refname, true
		} else {
			key, found = normed[pyLower(refname)]
		}
		var subdef *Node
		if !found {
			msg = d.reporter.error(fmt.Sprintf(`Undefined substitution referenced: "%s".`, refname), nil, baseNode(ref))
		} else {
			subdef = defs[key]
			if runeLen(subdef.astext()) > d.settings.lineLengthLimit {
				msg = d.reporter.error(fmt.Sprintf(`Substitution definition "%s" exceeds the line-length-limit.`, key), nil)
			}
		}
		if msg != nil {
			d.newProblematic(ref, msg)
			continue
		}
		parent := ref.Parent
		index := parent.index(ref)
		if subdef.has("ltrim") || subdef.has("trim") {
			if index > 0 && parent.Children[index-1].IsText() {
				parent.Children[index-1].Text = pyRstrip(parent.Children[index-1].Text)
			}
		}
		if subdef.has("rtrim") || subdef.has("trim") {
			if len(parent.Children) > index+1 && parent.Children[index+1].IsText() {
				parent.Children[index+1].Text = pyLstrip(parent.Children[index+1].Text)
			}
		}
		subdefCopy := subdef.deepcopy()
		circular := false
		for _, nestedRef := range subdefCopy.findTag("substitution_reference") {
			nestedName := normed[pyLower(nestedRef.str("refname"))]
			if containsStr(nested[nestedName], nestedName) {
				circular = true
				break
			}
			nested[nestedName] = append(nested[nestedName], key)
			origin[nestedRef] = ref
			subreflist = append(subreflist, nestedRef)
		}
		if circular {
			parent := ref.Parent
			if parent.Tag == "substitution_definition" {
				msg := d.reporter.error("Circular substitution definition detected:",
					[]*Node{newTextElem("literal_block", parent.Rawsource, parent.Rawsource)},
					atLine(parent.Line), baseNode(parent))
				parent.replaceSelf(msg)
			} else {
				refOrigin := ref
				for origin[refOrigin] != nil {
					refOrigin = origin[refOrigin]
				}
				msg := d.reporter.error(fmt.Sprintf(`Circular substitution definition referenced: "%s".`, refname),
					nil, baseNode(refOrigin))
				d.newProblematic(ref, msg)
			}
			continue
		}
		children := subdefCopy.Children
		if len(children) > 0 && !children[0].IsText() {
			children[0].updateBasicAtts(ref)
		}
		ref.Parent.replace(ref, children...)
		for _, node := range children {
			if node.is("reference", "footnote_reference", "citation_reference") {
				if node.has("refname") {
					d.noteRefname(node)
				}
			}
		}
	}
}

func xTargetNotes(d *Document, start *Node) {
	classes, _ := start.pending.details["class"].([]string)
	notes := map[string]*Node{}
	var nodelist []*Node
	makeFootnote := func(refuri string, refs []*Node) *Node {
		var footnote *Node
		var footnoteName string
		if fn, ok := notes[refuri]; ok {
			footnote = fn
			footnoteName = footnote.list("names")[0]
		} else {
			footnote = newElem("footnote", "")
			footnoteID := d.setID(footnote, nil, "")
			footnoteName = "TARGET_NOTE: " + footnoteID
			footnote.setAttr("auto", 1)
			footnote.Attrs["names"] = []string{footnoteName}
			p := newElem("paragraph", "")
			r := newTextElem("reference", "", refuri)
			r.setAttr("refuri", refuri)
			p.append(r)
			footnote.append(p)
			d.noteAutofootnote(footnote)
			d.noteExplicitTarget(footnote, footnote)
		}
		for _, ref := range refs {
			if ref.Tag == "target" {
				continue
			}
			refnode := newTextElem("footnote_reference", "", "")
			refnode.setAttr("refname", footnoteName)
			refnode.setAttr("auto", 1)
			refnode.appendTo("classes", classes...)
			d.noteAutofootnoteRef(refnode)
			d.noteFootnoteRef(refnode)
			index := ref.Parent.index(ref) + 1
			reflist := []*Node{refnode}
			if !d.settings.trimFootnoteRefSpace() {
				if len(classes) > 0 {
					in := newTextElem("inline", "", " ")
					in.Attrs["classes"] = append([]string{}, classes...)
					reflist = []*Node{in, refnode}
				} else {
					reflist = []*Node{newText(" "), refnode}
				}
			}
			ref.Parent.insertAll(index, reflist)
		}
		return footnote
	}
	for _, target := range d.Root.findTag("target") {
		if !target.has("refuri") {
			continue
		}
		var refs []*Node
		for _, name := range target.list("names") {
			refs = append(refs, d.refnames[name]...)
		}
		if len(refs) == 0 {
			continue
		}
		uri := target.str("refuri")
		footnote := makeFootnote(uri, refs)
		if _, ok := notes[uri]; !ok {
			notes[uri] = footnote
			nodelist = append(nodelist, footnote)
		}
	}
	for _, ref := range d.Root.findTag("reference") {
		if !ref.truthy("anonymous") {
			continue
		}
		if ref.has("refuri") {
			uri := ref.str("refuri")
			footnote := makeFootnote(uri, []*Node{ref})
			if _, ok := notes[uri]; !ok {
				notes[uri] = footnote
				nodelist = append(nodelist, footnote)
			}
		}
	}
	if len(nodelist) > 0 {
		nodelist[0].updateBasicAtts(start)
	}
	start.Parent.replace(start, nodelist...)
}

func xDanglingReferences(d *Document, _ *Node) {
	var walk func(n *Node)
	visit := func(node *Node) {
		if node.resolved || !node.has("refname") {
			return
		}
		refname := node.str("refname")
		id, known := d.nameids[refname]
		if id != "" {
			node.delAttr("refname")
			node.setAttr("refid", id)
			if t := d.ids[id]; t != nil {
				t.noteReferencedBy("", id)
			}
			node.resolved = true
			return
		}
		var msg *Node
		if known {
			msg = d.reporter.error(fmt.Sprintf(`Duplicate target name, cannot be used as a unique reference: "%s".`, refname),
				nil, baseNode(node))
		} else {
			var details []*Node
			if strings.Contains(refname, "<") || strings.Contains(refname, ">") {
				hint := "Did you want to embed a URI or alias?"
				if !strings.Contains(refname, "<") {
					hint += "\nOpening bracket missing."
				} else if !strings.Contains(refname, " <") {
					hint += "\nThe embedded reference must be preceded by whitespace."
				}
				if !strings.Contains(refname, ">") {
					hint += "\nClosing bracket missing."
				} else if !strings.HasSuffix(refname, ">") {
					hint += "\nThe embedded reference must be the last text before the end string."
				}
				if strings.Contains(refname, "< ") || strings.Contains(refname, " >") {
					hint += "\nWhitespace around the embedded reference is not allowed."
				}
				details = []*Node{newTextElem("paragraph", "", hint)}
			}
			msg = d.reporter.error(fmt.Sprintf(`Unknown target name: "%s".`, refname), details, baseNode(node))
		}
		msgid := d.setID(msg, nil, "")
		prb := newTextElem("problematic", node.Rawsource, node.Rawsource)
		prb.setAttr("refid", msgid)
		var prbid string
		if ids := node.list("ids"); len(ids) > 0 {
			prbid = ids[0]
		} else {
			prbid = d.setID(prb, nil, "")
		}
		msg.addBackref(prbid)
		node.replaceSelf(prb)
	}
	walk = func(n *Node) {
		if n.is("reference", "footnote_reference", "citation_reference") {
			visit(n)
		}
		for _, c := range append([]*Node{}, n.Children...) {
			walk(c)
		}
	}
	walk(d.Root)
	for _, target := range d.Root.findTag("target") {
		if !target.referenced {
			if target.truthy("anonymous") {
				continue
			}
			var naming string
			if n := target.list("names"); len(n) > 0 {
				naming = n[0]
			} else if ids := target.list("ids"); len(ids) > 0 {
				naming = ids[0]
			} else {
				naming = target.str("refid")
			}
			d.reporter.info(fmt.Sprintf(`Hyperlink target "%s" is not referenced.`, naming), nil, baseNode(target))
		}
	}
}
