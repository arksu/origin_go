import math
import utils

c = utils.coord
z = c(0, 0)
unit = c(1, 1)

sin = lambda a: math.sin(a * (2 * math.pi / 360))
cos = lambda a: math.cos(a * (2 * math.pi / 360))
def perc(o, a, r):
    return(c(o.x + int((cos(a) * r)), o.y - int((sin(a) * r))))

def simpletree(pts):
    gen = [[pts[0]]]
    for i in xrange(1, len(pts)):
        npt = [pts[i]]
        gen.append(npt)
        gen[i - 1].append(npt)
        npt.append(gen[i - 1])
    return gen

def gentree(oc, sz):
    pts = [[oc]]
    open = [(pts[0], utils.randoom(0, 360))]
    while len(open) > 0:
        #pt, ca = open.pop()
        pt, ca = open[0]
        open = open[1:]
        cc = pt[0]
        if(len(pts) < 4):
            cont = True
        else:
            cont = (utils.randoom(0, 2) == 0)
        if cont:
            num = utils.wrandoom([0, 4, 1])
            aa = 1.0 / (num + 1)
            sa = ca + int(num * 180 * aa)
            for i in xrange(num):
                na = utils.randoom(sa, sa - int(360 * aa))
                sa -= int(360 * aa)
                nr = utils.srandoom(sz, 0.1)
                nc = perc(cc, na, nr)
                npt = [nc]
                pt.append(npt)
                npt.append(pt)
                pts.append(npt)
                open.append((npt, na))
    return pts

def genoutline(pt, sz, first = True):
    gen = ()
    cc = pt[0]
    spts = pt[1:]
    if len(spts) == 1:
        spt = spts[0]
        sc = spt[0]
        r = utils.srandoom(sz, 0.2)
        sd = int(cc.dist(sc))
        if sd == 0: sd = 1
        gen += (c(cc.x - ((sc.y - cc.y) * r) / sd, cc.y + ((sc.x - cc.x) * r) / sd),)
        gen += (c(cc.x - ((sc.x - cc.x) * r) / sd, cc.y - ((sc.y - cc.y) * r) / sd),)
        gen += (c(cc.x + ((sc.y - cc.y) * r) / sd, cc.y - ((sc.x - cc.x) * r) / sd),)
        if first:
            gen += genoutline(spt, sz, False)
    elif len(spts) == 2:
        spt1 = spts[0]
        spt2 = spts[1]
        sc1 = spt1[0]
        sc2 = spt2[0]
        r = utils.srandoom(sz, 0.2)
        sd = int(sc1.dist(sc2))
        if sd == 0: sd = 1
        gen += (c(cc.x - ((sc1.y - sc2.y) * r) / sd, cc.y + ((sc1.x - sc2.x) * r) / sd),)
        gen += genoutline(spt2, sz, False)
        gen += (c(cc.x + ((sc1.y - sc2.y) * r) / sd, cc.y - ((sc1.x - sc2.x) * r) / sd),)
        if first:
            gen += genoutline(spt1, sz, False)
    else:
        for i in xrange(len(spts)):
            spt1 = spts[i]
            spt2 = spts[(i + 1) % len(spts)]
            sc1 = spt1[0]
            sc2 = spt2[0]
            r = utils.srandoom(sz, 0.2)
            sd = int(sc1.dist(sc2))
            if sd == 0: sd = 1
            gen += (c(cc.x - ((sc1.y - sc2.y) * r) / sd, cc.y + ((sc1.x - sc2.x) * r) / sd),)
            if first or i != len(spts) - 1:
                gen += genoutline(spt2, sz, False)
    return gen

class polypatch:
    def __init__(self, tpts, pts, swd, trn):
        self.trn = trn
        spts = self.mkspline(pts, swd)
        self.tpts = tpts
        self.pts = pts
        self.spts = spts
        self.calcsz()

    def calcsz(self):
        min = c(z)
        max = c(z)
        for pt in self.spts:
            if pt.x < min.x:
                min.x = pt.x
            if pt.y < min.y:
                min.y = pt.y
            if pt.x > max.x:
                max.x = pt.x
            if pt.y > max.y:
                max.y = pt.y
        asz = (max - min) + unit
        self.sz = asz
        self.extent = utils.corn2area(min, max)
        
    def recenter(self):
        mp = self.extent.ul + (self.sz / 2)
        self.tpts = [pt - mp for pt in self.tpts]
        self.pts = [pt - mp for pt in self.pts]
        self.spts = [pt - mp for pt in self.spts]
        self.calcsz()
    
    def mkspline(self, pts, sz):
        ret = []
        d = pts[0] - pts[-1]
        for i in xrange(len(pts)):
            pt1 = pts[i]
            pt2 = pts[(i + 1) % len(pts)]
            dist = int(d.dist(z))
            if dist > 0:
                d = d * sz / int(d.dist(z))
            else:
                d = c(0, 0)
            d, spts = utils.spline(pt1, pt2, d, int(pt1.dist(pt2)) / 3)
            for spt in spts:
                spt = spt.int()
                ret.append(spt)
        ret.append(pts[0])
        return ret

    def draw(self, img, cc, jag = 0, cpp = None, gdid = None, debug = None):
        if gdid is None:
            gdid = self.trn.sgid
        oc = None
        opt = None
        for pt in self.spts:
            dpt = pt
            if jag != 0:
                dpt = c(pt.x + utils.randoom(-jag, jag + 1), pt.y + utils.randoom(-jag, jag + 1))
            if oc is not None:
                img.line(oc + cc, dpt + cc, gdid)
                if cpp is not None and utils.randoom(0, cpp) == 0:
                    sz = utils.randoom(1, 15)
                    r = utils.randoom(5, 7)
                    dist = opt.dist(pt)
                    if dist >= 1:
                        origin = c(opt.x + ((pt.y - opt.y) * r) / dist, opt.y - ((pt.x - opt.x) * r) / dist)
                        utils.modpx(mkcpp(origin.int() + cc, sz, gdid), img)
            else:
                fst = dpt
            oc = dpt
            opt = pt
        img.line(oc + cc, fst + cc, gdid)
        for pt in self.tpts:
            tc = pt + cc
            if debug is None:
                img.fillToBorder(tc, gdid, gdid)
            else:
                img.setPixel(tc, debug)

class treepatch(polypatch):
    def __init__(self, tpts, trn, wd):
        tcpts = [pt[0] for pt in tpts]
        polypatch.__init__(self, tcpts, genoutline(tpts[0], wd), wd, trn)

def debugpatch(patch, out):
    import gd
    img = gd.image(patch.sz)
    bg = img.colorAllocate((0, 0, 0))
    fg = img.colorAllocate((255, 255, 255))
    ptc = img.colorAllocate((255, 0, 128))
    patch.draw(img, patch.sz / 2, gdid = fg, debug = ptc)
    img.writePng(out + "c.png")

    img = gd.image(patch.sz)
    bg = img.colorAllocate((0, 0, 0))
    fg = img.colorAllocate((255, 255, 255))
    patch.draw(img, patch.sz / 2, gdid = fg)
    img.writePng(out + "f.png")

def randpatch(sz, trn, wd = None):
    if wd is None: wd = sz / 2
    tree = gentree(z, sz)
    ret = treepatch(tree, trn, wd)
    ret.tree = tree
    ret.wd = wd
    ret.recenter()
    return ret

def mkcpp(cc, sz, id):
    ret = []
    drawn = []
    open = [cc]
    min = c(cc)
    max = c(cc)
    while sz > 0 and len(open) > 0:
        cur = open.pop()
        if cur.x < min.x : min.x = cur.x
        if cur.y < min.y : min.y = cur.y
        if cur.x > max.x : max.x = cur.x
        if cur.y > max.y : max.y = cur.y
        for i in [1, 2, 3]:
            mod = (utils.randoom(0, 2) * 2) - 1
            if(utils.randoom(0, 2) == 0):
                ncc = c(cur.x + mod, cur.y)
            else:
                ncc = c(cur.x, cur.y + mod)
            if ncc != cur and not ncc in drawn and not ncc in open:
                open.insert(0, ncc)
        drawn.append(cur)
        ret.append((cur, id))
        sz -= 1
    return ret
